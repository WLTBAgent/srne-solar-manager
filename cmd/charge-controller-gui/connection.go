package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2/dialog"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// onConnectButton handles the connect/disconnect button. Main goroutine.
func (g *App) onConnectButton() {
	switch g.getState() {
	case stateConnected:
		g.disconnect()
	case stateDisconnected:
		g.connect()
	}
}

// connect spawns the connection attempt in the background so the UI
// never blocks on serial I/O.
func (g *App) connect() {
	g.mu.Lock()
	if g.state != stateDisconnected {
		g.mu.Unlock()
		return
	}
	g.state = stateConnecting
	g.mu.Unlock()
	g.setState(stateConnecting)

	cfg, ok := g.connectionConfig()
	if !ok {
		g.setState(stateDisconnected)
		dialog.ShowError(fmt.Errorf("invalid device ID %q", g.deviceEntry.Text), g.window)
		return
	}

	go func() {
		g.busyStart()
		c := controller.NewController(cfg)
		err := c.Connect()
		g.busyEnd()

		if err != nil {
			// The logical state lives outside the UI thread so that
			// background workers see transitions immediately; only the
			// visuals are dispatched with fyne.Do.
			g.mu.Lock()
			g.state = stateDisconnected
			g.mu.Unlock()
			ui(func() {
				g.setState(stateDisconnected)
				g.reportError("Connect failed: %v", err)
				dialog.ShowError(fmt.Errorf("connect to %s failed:\n%v", cfg.Port, err), g.window)
			})
			return
		}

		g.setConn(c)
		g.mu.Lock()
		g.state = stateConnected
		g.mu.Unlock()
		ui(func() {
			g.setState(stateConnected)
			g.setStatus("Connected to %s", cfg.Port)
			g.log("Connected to %s (baud %d, device ID %d)", cfg.Port, cfg.Baud, cfg.Device)
			// Remember this connection for the next launch.
			prefs := g.app.Preferences()
			prefs.SetString("port", cfg.Port)
			prefs.SetString("baud", strconv.Itoa(cfg.Baud))
			prefs.SetString("device", strconv.Itoa(cfg.Device))
		})

		// Load every tab once, sequentially, BEFORE the poll loop
		// starts: the initial one-shot reads get a quiet bus, which
		// matters on gateways that drop requests when busy.
		g.busyStart()
		g.setting.loadValues(c)
		g.info.load(c)
		g.stats.load(c)
		g.faults.load(c)
		g.busyEnd()
		ui(func() { g.startMonitorLoop() })
	}()
}

// disconnect tears the connection down without blocking the UI.
func (g *App) disconnect() {
	c := g.takeConn()
	if c == nil {
		return
	}
	g.stopMonitorLoop()
	g.setState(stateDisconnected)
	g.setStatus("Disconnected")
	g.log("Disconnected")
	g.dash.markDisconnected()

	go func() {
		g.busyStart()
		c.Close()
		g.busyEnd()
	}()
}

// connectionConfig reads the connection bar. Main goroutine only.
func (g *App) connectionConfig() (controller.Connection, bool) {
	deviceID, err := strconv.Atoi(g.deviceEntry.Text)
	if err != nil || deviceID < 1 || deviceID > 247 {
		return controller.Connection{}, false
	}
	baud, _ := strconv.Atoi(g.baudSelect.Selected)
	return controller.Connection{
		Port:    g.portEntry.Text,
		Device:  deviceID,
		Baud:    baud,
		Timeout: 1500 * time.Millisecond,
	}, true
}

// ─── Monitor loop ───

func (g *App) startMonitorLoop() {
	g.stopMonitorLoop() // safety: never two loops
	stop := make(chan struct{})
	g.mu.Lock()
	g.stopMon = stop
	g.mu.Unlock()
	go g.monitorLoop(stop)
}

func (g *App) stopMonitorLoop() {
	g.mu.Lock()
	if g.stopMon != nil {
		close(g.stopMon)
		g.stopMon = nil
	}
	g.mu.Unlock()
}

func (g *App) pollInterval() time.Duration {
	return time.Duration(g.pollMillis.Load()) * time.Millisecond
}

// monitorLoop polls the dashboard registers until stop is closed.
// All serial I/O happens here; UI updates are dispatched with fyne.Do.
//
// Blocks that fail repeatedly are skipped for a cool-off period: one
// dead area (e.g. an unreachable inverter section behind the internal
// gateway) would otherwise burn the poll timeout every cycle and keep
// the rest of the bus busy.
func (g *App) monitorLoop(stop chan struct{}) {
	const (
		maxConsecutiveFails = 3
		blockCooldown       = time.Minute
	)
	blocks := controller.PlanRegisterBlocks(dashboardRegs(), controller.MaxBlockCount, controller.MaxBlockGap)
	failures := map[uint16]int{}
	skipUntil := map[uint16]time.Time{}

	for {
		select {
		case <-stop:
			return
		case <-time.After(g.pollInterval()):
		}
		if g.paused.Load() {
			continue
		}
		conn := g.getConn()
		if conn == nil {
			return
		}

		g.busyStart()
		values := make(map[uint16][]uint16, len(dashboardAddrs))
		var msgs []string
		now := time.Now()
		for _, b := range blocks {
			if until, skip := skipUntil[b.Start]; skip {
				if now.Before(until) {
					continue // cooling off
				}
				delete(skipUntil, b.Start) // cool-off over: give it another chance
				failures[b.Start] = 0
			}

			v, err := conn.ReadBlock(b)
			if err != nil {
				failures[b.Start]++
				switch {
				case failures[b.Start] == maxConsecutiveFails:
					skipUntil[b.Start] = now.Add(blockCooldown)
					msgs = append(msgs, fmt.Sprintf(
						"block 0x%04X keeps failing (%v); skipping it for %s",
						b.Start, err, blockCooldown))
				case failures[b.Start] < maxConsecutiveFails:
					msgs = append(msgs, fmt.Sprintf("block 0x%04X (%d regs): %v", b.Start, b.Count, err))
				}
				continue
			}
			failures[b.Start] = 0
			for addr, words := range v {
				values[addr] = words
			}
		}
		g.busyEnd()

		ui(func() {
			if g.getConn() != conn {
				return // disconnected while reading
			}
			g.dash.apply(values)
			if len(msgs) > 0 {
				g.reportErrors(msgs)
			} else {
				g.setStatus("Updated %s", time.Now().Format("15:04:05"))
			}
		})
	}
}

// regsByAddrs resolves addresses to registers, skipping unknown ones.
func regsByAddrs(addrs []uint16) []*controller.Register {
	regs := make([]*controller.Register, 0, len(addrs))
	for _, addr := range addrs {
		if r := controller.FindRegisterByAddr(addr); r != nil {
			regs = append(regs, r)
		}
	}
	return regs
}

// async runs fn in a background goroutine with the busy indicator.
func (g *App) async(fn func()) {
	go func() {
		g.busyStart()
		defer g.busyEnd()
		fn()
	}()
}
