package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/wltechblog/charge-controller/internal/controller"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printHelp()
		return
	}

	command := args[0]
	rest := args[1:]

	switch command {
	case "list":
		cmdList(rest)
	case "status", "read", "read-all":
		cmdStatus(rest)
	case "get":
		cmdGet(rest)
	case "set":
		cmdSet(rest)
	case "control":
		cmdControl(rest)
	case "monitor":
		cmdMonitor(rest)
	case "dump":
		cmdDump(rest)
	case "version":
		fmt.Printf("charge-controller %s\n", controller.Version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`
╔══════════════════════════════════════════════════════════════════╗
║          Charge Controller Modbus Tool v` + controller.Version + `                    ║
║          Integrated Inverter Controller V1.3                      ║
╚══════════════════════════════════════════════════════════════════╝

USAGE:
  charge-controller <command> [options] [-port /dev/ttyUSB0] [-device 1] [-baud 9600]

GLOBAL OPTIONS:
  -port <path>     Serial device (default: /dev/ttyUSB0)
  -device <id>     Modbus slave ID (default: 1)
  -baud <rate>     Baud rate (default: 9600)

COMMANDS:

  list [--configurable] [--category <name>] [--json]
  status [--json]
  get <address|name> [--json]
  set <address|name> <value>
  control <action>
  monitor [--interval 5s]
  dump [--start 0x0000] [--count 65535] [--json]
  version

EXAMPLES:
  charge-controller list --configurable
  charge-controller status
  charge-controller set 0xE001 80
  charge-controller control power-on
`)
}

func parseGlobalFlags(args []string) ([]string, controller.Connection) {
	cfg := controller.Connection{Port: "/dev/ttyUSB0", Device: 1, Baud: 9600}
	var filtered []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-port":
			if i+1 < len(args) {
				cfg.Port = args[i+1]
				i++
			}
		case "-device":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cfg.Device)
				i++
			}
		case "-baud":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cfg.Baud)
				i++
			}
		default:
			filtered = append(filtered, args[i])
		}
	}
	return filtered, cfg
}

func connect(cfg controller.Connection) *controller.Controller {
	c := controller.NewController(cfg)
	if err := c.Connect(); err != nil {
		log.Fatal(err)
	}
	return c
}

func cmdList(args []string) {
	args, _ = parseGlobalFlags(args)

	configurableOnly := false
	categoryFilter := ""
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--configurable":
			configurableOnly = true
		case "--category":
			if i+1 < len(args) {
				categoryFilter = args[i+1]
				i++
			}
		case "--json":
			jsonOut = true
		}
	}

	var regs []controller.Register
	if configurableOnly {
		regs = controller.ConfigurableRegisters()
	} else {
		regs = controller.AllRegisters
	}

	if jsonOut {
		out, _ := json.MarshalIndent(regs, "", "  ")
		fmt.Println(string(out))
		return
	}

	cats := make(map[string][]controller.Register)
	for _, r := range regs {
		if categoryFilter != "" && !strings.Contains(strings.ToLower(r.Category), strings.ToLower(categoryFilter)) {
			continue
		}
		cats[r.Category] = append(cats[r.Category], r)
	}

	sortedCats := make([]string, 0, len(cats))
	for k := range cats {
		sortedCats = append(sortedCats, k)
	}
	sort.Strings(sortedCats)

	for _, cat := range sortedCats {
		pad := 60 - len(cat)
		if pad < 1 {
			pad = 1
		}
		fmt.Printf("\n╔─ %s %s╗\n", cat, strings.Repeat("─", pad))
		for _, r := range cats[cat] {
			access := ""
			switch r.Access {
			case "RW":
				access = " [RW]"
			case "W":
				access = " [W] "
			default:
				access = " [R] "
			}
			fmt.Printf("║ 0x%04X  %-44s%s\n", r.Address, r.Name, access)
			if r.Unit != "" || r.Min != 0 || r.Max != 0 {
				detail := ""
				if r.Unit != "" {
					detail += fmt.Sprintf(" unit=%s", r.Unit)
				}
				if r.Min != 0 || r.Max != 0 {
					detail += fmt.Sprintf(" range=%.1f–%.1f", r.Min, r.Max)
				}
				if r.Default != 0 {
					detail += fmt.Sprintf(" default=%.1f", r.Default)
				}
				fmt.Printf("║         %s\n", detail)
			}
			if len(r.Enum) > 0 {
				for _, k := range controller.EnumSortedKeys(r.Enum) {
					fmt.Printf("║         %d = %s\n", k, r.Enum[k])
				}
			}
			if r.Description != "" {
				fmt.Printf("║         └ %s\n", r.Description)
			}
		}
	}
	fmt.Println()
}

func cmdStatus(args []string) {
	args, cfg := parseGlobalFlags(args)
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		}
	}

	c := connect(cfg)
	defer c.Close()

	var values []*controller.Reading
	var errs []error

	for _, addr := range controller.StatusAddresses {
		reg := controller.FindRegisterByAddr(addr)
		if reg == nil {
			continue
		}
		r, err := c.ReadReading(reg)
		if err != nil {
			errs = append(errs, fmt.Errorf("0x%04X: %w", addr, err))
			continue
		}
		values = append(values, r)
		time.Sleep(10 * time.Millisecond)
	}

	if jsonOut {
		out, _ := json.MarshalIndent(values, "", "  ")
		fmt.Println(string(out))
		return
	}

	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              CHARGE CONTROLLER — LIVE STATUS                 ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	categories := []string{"Controller Data", "Inverter Data"}
	for _, cat := range categories {
		hasData := false
		for _, v := range values {
			if v.Category == cat {
				hasData = true
				break
			}
		}
		if !hasData {
			continue
		}
		fmt.Printf("\n── %s ──\n", cat)
		for _, v := range values {
			if v.Category != cat {
				continue
			}
			fmt.Printf("  %-36s %s\n", v.Name+":", v.Decoded)
		}
	}

	if len(errs) > 0 {
		fmt.Println("\n⚠ Read errors:")
		for _, e := range errs {
			fmt.Printf("  %v\n", e)
		}
	}
	fmt.Println()
}

func cmdGet(args []string) {
	args, cfg := parseGlobalFlags(args)
	if len(args) == 0 {
		log.Fatal("Usage: get <address|name>")
	}

	jsonOut := false
	query := args[0]
	for _, a := range args[1:] {
		if a == "--json" {
			jsonOut = true
		}
	}

	reg := controller.FindRegister(query)
	if reg == nil {
		log.Fatalf("Register not found: %s", query)
	}

	c := connect(cfg)
	defer c.Close()

	raw, err := c.ReadRegister(reg)
	if err != nil {
		log.Fatalf("Read failed: %v", err)
	}

	decoded := controller.DecodeValue(reg, raw)

	if jsonOut {
		v := controller.Reading{
			Address: reg.Address, Name: reg.Name, Raw: raw[0],
			Decoded: decoded, Unit: reg.Unit, Category: reg.Category,
		}
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
		return
	}

	fmt.Printf("  Address:  0x%04X\n", reg.Address)
	fmt.Printf("  Name:     %s\n", reg.Name)
	fmt.Printf("  Access:   %s\n", reg.Access)
	fmt.Printf("  Raw:      0x%04X (%d)\n", raw[0], raw[0])
	fmt.Printf("  Value:    %s\n", decoded)
	if reg.Unit != "" {
		fmt.Printf("  Unit:     %s\n", reg.Unit)
	}
	if reg.Description != "" {
		fmt.Printf("  Note:     %s\n", reg.Description)
	}
	if len(reg.Enum) > 0 {
		fmt.Println("  Options:")
		for _, k := range controller.EnumSortedKeys(reg.Enum) {
			marker := "  "
			if k == raw[0] {
				marker = "▶ "
			}
			fmt.Printf("    %s%d = %s\n", marker, k, reg.Enum[k])
		}
	}
	fmt.Println()
}

func cmdSet(args []string) {
	args, cfg := parseGlobalFlags(args)
	if len(args) < 2 {
		log.Fatal("Usage: set <address|name> <value>")
	}

	reg := controller.FindRegister(args[0])
	if reg == nil {
		log.Fatalf("Register not found: %s", args[0])
	}
	if reg.Access != "RW" && reg.Access != "W" {
		log.Fatalf("Register %s is read-only", reg.Name)
	}

	raw, err := controller.ParseValue(args[1], reg)
	if err != nil {
		log.Fatalf("Invalid value: %v", err)
	}
	if err := controller.ValidateValue(reg, raw); err != nil {
		log.Fatalf("Validation failed: %v", err)
	}

	c := connect(cfg)
	defer c.Close()

	fmt.Printf("Writing 0x%04X (%d) to %s (0x%04X)...\n", raw, raw, reg.Name, reg.Address)
	if err := c.WriteRegister(reg.Address, raw); err != nil {
		log.Fatalf("Write failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	readRaw, err := c.ReadRegister(reg)
	if err != nil {
		fmt.Printf("⚠ Write succeeded but readback failed: %v\n", err)
		return
	}
	fmt.Printf("✓ Readback: 0x%04X (%d) → %s\n", readRaw[0], readRaw[0], controller.DecodeValue(reg, readRaw))
}

func cmdControl(args []string) {
	args, cfg := parseGlobalFlags(args)
	if len(args) == 0 {
		log.Fatal("Usage: control <action>")
	}

	act, ok := controller.ControlActions[args[0]]
	if !ok {
		log.Fatalf("Unknown control action: %s", args[0])
	}

	c := connect(cfg)
	defer c.Close()

	fmt.Printf("Sending: %s (0x%04X → 0x%04X)...\n", act.Desc, act.Val, act.Addr)
	if err := c.WriteRegister(act.Addr, act.Val); err != nil {
		log.Fatalf("Write failed: %v", err)
	}
	fmt.Println("✓ Sent successfully")
}

func cmdMonitor(args []string) {
	args, cfg := parseGlobalFlags(args)

	interval := 5 * time.Second
	for i := 0; i < len(args); i++ {
		if args[i] == "--interval" && i+1 < len(args) {
			d, err := time.ParseDuration(args[i+1])
			if err == nil {
				interval = d
			}
			i++
		}
	}

	c := connect(cfg)
	defer c.Close()

	addrs := []uint16{0x0100, 0x0101, 0x0102, 0x0107, 0x0108, 0x0109, 0x010B, 0x0210, 0x0213, 0x0216, 0x021B, 0x0223}

	for {
		fmt.Printf("\n┌─ %s ────────────────────────────\n", time.Now().Format("15:04:05"))
		for _, addr := range addrs {
			reg := controller.FindRegisterByAddr(addr)
			if reg == nil {
				continue
			}
			raw, err := c.ReadRegister(reg)
			if err != nil {
				fmt.Printf("│ %-30s ERROR: %v\n", reg.Name, err)
				continue
			}
			fmt.Printf("│ %-30s %s\n", reg.Name, controller.DecodeValue(reg, raw))
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Println("└──────────────────────────────────")
		time.Sleep(interval)
	}
}

func cmdDump(args []string) {
	args, cfg := parseGlobalFlags(args)

	startAddr := uint16(0x0000)
	count := uint16(0xFFFF)
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--start":
			if i+1 < len(args) {
				a, err := controller.ParseAddress(args[i+1])
				if err == nil {
					startAddr = a
				}
				i++
			}
		case "--count":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &count)
				i++
			}
		case "--json":
			jsonOut = true
		}
	}

	c := connect(cfg)
	defer c.Close()

	type dumpEntry struct {
		Address uint16 `json:"address"`
		Value   uint16 `json:"value"`
	}
	var entries []dumpEntry
	var currentAddr uint16 = startAddr
	endAddr := startAddr + count

	for currentAddr < endAddr {
		chunkSize := uint16(20)
		if currentAddr+chunkSize > endAddr {
			chunkSize = endAddr - currentAddr
		}
		results, err := c.ReadChunk(currentAddr, chunkSize)
		if err != nil {
			if !jsonOut {
				fmt.Printf("0x%04X: [error: %v]\n", currentAddr, err)
			}
			currentAddr += chunkSize
			continue
		}
		for i, v := range results {
			addr := currentAddr + uint16(i)
			if jsonOut {
				entries = append(entries, dumpEntry{addr, v})
			} else {
				if v != 0 {
					fmt.Printf("0x%04X: 0x%04X (%d)\n", addr, v, v)
				}
			}
		}
		currentAddr += chunkSize
		time.Sleep(10 * time.Millisecond)
	}

	if jsonOut {
		out, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(out))
	}
}
