// Command devsim runs a virtual SRNE inverter charge controller on a
// pseudo-terminal so the GUI can be exercised without hardware.
//
// It creates a PTY pair, serves Modbus RTU on the master end and prints
// the slave path to point the GUI (or CLI) at, e.g.:
//
//	./charge-controller-gui   (Port: /dev/pts/3)
//	./charge-controller status -port /dev/pts/3
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wltechblog/charge-controller/internal/devsim"
)

func main() {
	slaveID := flag.Int("device", 1, "Modbus slave ID to answer as")
	tick := flag.Duration("tick", 2*time.Second, "live-value jitter interval")
	deny := flag.String("deny", "", "comma-separated hex addresses that never answer (e.g. 0x0204)")
	flag.Parse()

	master, slavePath, err := devsim.OpenPTY()
	if err != nil {
		log.Fatalf("devsim: %v", err)
	}

	d := devsim.New(byte(*slaveID))
	for _, part := range strings.Split(*deny, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(part), "0x"), 16, 16)
		if err != nil {
			log.Fatalf("devsim: bad -deny address %q: %v", part, err)
		}
		d.Deny = append(d.Deny, uint16(v))
	}
	go func() {
		if err := d.Serve(master); err != nil {
			log.Printf("devsim: serve ended: %v", err)
			os.Exit(0)
		}
	}()

	ticker := time.NewTicker(*tick)
	go func() {
		for range ticker.C {
			d.Tick()
		}
	}()

	fmt.Printf("Virtual SRNE controller listening.\n")
	fmt.Printf("  Port:  %s\n", slavePath)
	fmt.Printf("  Slave: %d\n", *slaveID)
	fmt.Printf("Point the GUI or CLI at the port above. Ctrl-C to quit.\n")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\nShutting down.")
	ticker.Stop()
	master.Close()
}
