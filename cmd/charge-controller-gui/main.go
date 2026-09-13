// Command charge-controller-gui is a Fyne desktop GUI for monitoring and
// configuring SRNE-based all-in-one solar charge controllers over Modbus RTU.
//
// Threading model: only the main (UI) goroutine touches widgets. Every
// serial operation runs in a background goroutine and publishes results
// to the UI via fyne.Do. The Controller itself serializes bus access.
package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

type connState int

const (
	stateDisconnected connState = iota
	stateConnecting
	stateConnected
)

// App holds all GUI state.
type App struct {
	app    fyne.App
	window fyne.Window
	opts   LaunchOptions

	mu    sync.Mutex
	conn  *controller.Controller
	state connState

	stopMon    chan struct{} // closed to stop the monitor loop
	paused     atomic.Bool
	pollMillis atomic.Int64
	busyCount  atomic.Int32

	// connection bar
	portEntry   *widget.SelectEntry
	baudSelect  *widget.Select
	deviceEntry *widget.Entry
	pollSelect  *widget.Select
	connBtn     *widget.Button
	connDot     *widget.RichText
	activity    *widget.Activity
	statusText  *widget.Label

	tabs *container.AppTabs

	dash    *dashboardPanel
	setting *settingsPanel
	control *controlPanel
	stats   *statsPanel
	faults  *faultsPanel
	info    *infoPanel
}

// LaunchOptions customize startup; flags and saved preferences feed it.
type LaunchOptions struct {
	Port        string
	Baud        int
	Device      int
	Tab         int
	AutoConnect bool
}

func main() {
	port := flag.String("port", "", "serial port to prefill (e.g. /dev/ttyUSB0)")
	baud := flag.Int("baud", 0, "baud rate to prefill")
	device := flag.Int("device", 0, "Modbus device ID to prefill")
	tab := flag.Int("tab", 0, "tab index to open on (0=Dashboard … 5=Info)")
	connect := flag.Bool("connect", false, "connect immediately on startup")
	flag.Parse()

	// Declare metadata up front: it gives Preferences a stable ID and
	// tells Fyne this app is migrated to the fyne.Do threading model.
	app.SetMetadata(fyne.AppMetadata{
		ID:         "io.github.wltechblog.srne-solar-manager",
		Name:       "SRNE Charge Controller",
		Version:    controller.Version,
		Migrations: map[string]bool{"fyneDo": true},
	})

	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("SRNE Charge Controller")
	w.Resize(fyne.NewSize(1080, 720))

	g := newApp(a, w, LaunchOptions{
		Port:        *port,
		Baud:        *baud,
		Device:      *device,
		Tab:         *tab,
		AutoConnect: *connect,
	})
	w.SetContent(g.buildRoot())
	w.SetOnClosed(g.onClosed)

	if g.opts.Tab > 0 {
		n := g.opts.Tab
		go func() {
			time.Sleep(200 * time.Millisecond)
			ui(func() { g.tabs.SelectIndex(n) })
		}()
	}
	if g.opts.AutoConnect {
		go func() {
			time.Sleep(300 * time.Millisecond) // let the window appear first
			ui(g.connect)
		}()
	}
	w.ShowAndRun()
}

// newApp constructs the full UI. It is separated from main so tests can
// build the widget tree with fyne's test app.
func newApp(a fyne.App, w fyne.Window, opts LaunchOptions) *App {
	g := &App{app: a, window: w, opts: opts}
	g.pollMillis.Store(2000)

	g.dash = newDashboard(g)
	g.setting = newSettings(g)
	g.control = newControl(g)
	g.stats = newStats(g)
	g.faults = newFaults(g)
	g.info = newInfo(g)

	g.tabs = container.NewAppTabs(
		container.NewTabItem("Dashboard", g.dash.canvas()),
		container.NewTabItem("Settings", g.setting.canvas()),
		container.NewTabItem("Control", g.control.canvas()),
		container.NewTabItem("Statistics", g.stats.canvas()),
		container.NewTabItem("Faults", g.faults.canvas()),
		container.NewTabItem("Info", g.info.canvas()),
	)
	g.tabs.SetTabLocation(container.TabLocationTop)
	return g
}

func (g *App) buildRoot() fyne.CanvasObject {
	return container.NewBorder(
		g.buildConnBar(),
		g.buildStatusBar(),
		nil, nil,
		g.tabs,
	)
}

// ─── Connection bar & status bar ───

func (g *App) buildConnBar() fyne.CanvasObject {
	// Restore the last used connection, with launch flags taking priority.
	prefs := g.app.Preferences()
	port := prefs.StringWithFallback("port", "/dev/ttyUSB0")
	baud := prefs.StringWithFallback("baud", "9600")
	device := prefs.StringWithFallback("device", "1")
	poll := prefs.StringWithFallback("poll", "2s")
	if g.opts.Port != "" {
		port = g.opts.Port
	}
	if g.opts.Baud > 0 {
		baud = strconv.Itoa(g.opts.Baud)
	}
	if g.opts.Device > 0 {
		device = strconv.Itoa(g.opts.Device)
	}

	g.portEntry = widget.NewSelectEntry(nil)
	g.portEntry.SetPlaceHolder("Serial port (ttyUSB*/ttyACM*)")
	g.portEntry.SetText(port)
	g.refreshPortOptions()

	refreshPorts := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), g.refreshPortOptions)

	g.baudSelect = widget.NewSelect([]string{"4800", "9600", "19200", "38400", "57600", "115200"}, nil)
	g.baudSelect.SetSelected(baud)

	g.deviceEntry = widget.NewEntry()
	g.deviceEntry.SetText(device)

	g.pollSelect = widget.NewSelect([]string{"1s", "2s", "5s", "10s"}, func(s string) {
		if d, err := time.ParseDuration(s); err == nil {
			g.pollMillis.Store(d.Milliseconds())
			g.app.Preferences().SetString("poll", s)
		}
	})
	g.pollSelect.SetSelected(poll)

	g.connDot = widget.NewRichText()
	g.setConnDot(theme.ColorNameError, "Disconnected")

	g.connBtn = widget.NewButton("Connect", g.onConnectButton)

	right := container.NewHBox(
		widget.NewLabel("Baud:"), g.baudSelect,
		widget.NewLabel("ID:"), g.deviceEntry,
		widget.NewLabel("Poll:"), g.pollSelect,
		g.connBtn,
		g.connDot,
	)
	// Port entry is the Border center so it takes all remaining width;
	// the refresh button re-scans for plugged-in adapters.
	portBox := container.NewBorder(nil, nil, nil, refreshPorts, g.portEntry)
	return container.NewPadded(container.NewBorder(nil, nil, widget.NewLabel("Port:"), right, portBox))
}

// refreshPortOptions repopulates the port drop-down with detected
// serial devices, keeping whatever is currently typed. Main goroutine.
func (g *App) refreshPortOptions() {
	g.portEntry.SetOptions(mergePortOption(detectSerialPorts(), g.portEntry.Text))
}

func (g *App) buildStatusBar() fyne.CanvasObject {
	g.activity = widget.NewActivity()
	g.activity.Hide()

	g.statusText = widget.NewLabel("Not connected")
	g.statusText.TextStyle = fyne.TextStyle{Italic: true}
	g.statusText.Wrapping = fyne.TextTruncate

	version := widget.NewLabel("v" + controller.Version)
	version.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewBorder(nil, nil, container.NewHBox(g.activity), version, container.NewPadded(g.statusText))
}

func (g *App) onClosed() {
	g.stopMonitorLoop()
	if c := g.getConn(); c != nil {
		c.Close()
	}
}

// ─── State handling ───

func (g *App) getState() connState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func (g *App) setConn(c *controller.Controller) {
	g.mu.Lock()
	g.conn = c
	g.mu.Unlock()
}

// takeConn atomically clears and returns the connection.
func (g *App) takeConn() *controller.Controller {
	g.mu.Lock()
	c := g.conn
	g.conn = nil
	g.state = stateDisconnected
	g.mu.Unlock()
	return c
}

func (g *App) getConn() *controller.Controller {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.conn
}

// setConnDot updates the connection indicator text with a themed color.
// Main goroutine only.
func (g *App) setConnDot(color fyne.ThemeColorName, text string) {
	style := widget.RichTextStyle{Inline: true, TextStyle: fyne.TextStyle{Bold: true}, ColorName: color}
	g.connDot.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: "● ", Style: style},
		&widget.TextSegment{Text: text, Style: style},
	}
	g.connDot.Refresh()
}

// setState transitions the visible connection state. Main goroutine only.
func (g *App) setState(s connState) {
	g.mu.Lock()
	g.state = s
	g.mu.Unlock()

	switch s {
	case stateConnecting:
		g.setConnDot(theme.ColorNameWarning, "Connecting…")
		g.connBtn.SetText("Connect")
		g.connBtn.Disable()
	case stateConnected:
		g.setConnDot(theme.ColorNameSuccess, "Connected")
		g.connBtn.SetText("Disconnect")
		g.connBtn.Enable()
	default:
		g.setConnDot(theme.ColorNameError, "Disconnected")
		g.connBtn.SetText("Connect")
		g.connBtn.Enable()
	}
}

// ─── Cross-goroutine helpers ───

// ui runs fn on the main/UI goroutine (Fyne widgets are not thread-safe).
func ui(fn func()) {
	fyne.Do(fn)
}

func (g *App) busyStart() {
	if g.busyCount.Add(1) == 1 {
		ui(func() { g.activity.Start(); g.activity.Show() })
	}
}

func (g *App) busyEnd() {
	if g.busyCount.Add(-1) <= 0 {
		g.busyCount.Store(0)
		ui(func() { g.activity.Stop(); g.activity.Hide() })
	}
}

func (g *App) setStatus(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ui(func() { g.statusText.SetText(msg) })
}

// reportError surfaces a problem in the status bar and prints it to
// stderr (with a timestamp) so it can be copied from the terminal the
// app was launched from.
func (g *App) reportError(format string, args ...interface{}) {
	g.reportErrors([]string{fmt.Sprintf(format, args...)})
}

// reportErrors is reportError for multi-error read batches: every
// message goes to stderr; the status bar shows the first plus a count.
func (g *App) reportErrors(msgs []string) {
	for _, m := range msgs {
		log.Printf("%s", m)
	}
	ui(func() {
		msg := msgs[0]
		if len(msgs) > 1 {
			msg += fmt.Sprintf("  (+%d more, see stderr)", len(msgs)-1)
		}
		g.statusText.SetText(msg)
	})
}

// log appends a timestamped line to the activity log on the Control tab.
func (g *App) log(format string, args ...interface{}) {
	g.control.appendLog(fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...)))
}
