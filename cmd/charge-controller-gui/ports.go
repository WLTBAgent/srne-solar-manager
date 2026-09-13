package main

import (
	"path/filepath"
	"sort"
)

// serialPortPatterns are the device-node globs offered in the port
// drop-down: USB-to-serial adapters (ttyUSB) and USB CDC devices
// such as built-in USB ports on controller boards (ttyACM).
var serialPortPatterns = []string{"/dev/ttyUSB*", "/dev/ttyACM*"}

// detectSerialPorts returns existing device nodes matching the
// patterns, sorted; used to populate the port drop-down.
func detectSerialPorts() []string {
	return globPorts(serialPortPatterns)
}

func globPorts(patterns []string) []string {
	seen := map[string]bool{}
	var ports []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				ports = append(ports, m)
			}
		}
	}
	sort.Strings(ports)
	return ports
}

// mergePortOption prepends the currently entered port when it is not
// among the detected ones (e.g. a /dev/pts path for the simulator or a
// device that was just unplugged), so the drop-down never hides it.
func mergePortOption(options []string, current string) []string {
	if current == "" {
		return options
	}
	for _, o := range options {
		if o == current {
			return options
		}
	}
	return append([]string{current}, options...)
}
