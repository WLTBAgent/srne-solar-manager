package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobPorts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ttyUSB1", "ttyUSB0", "ttyACM3", "ttyACM0", "ttyS0", "other"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ports := globPorts([]string{
		filepath.Join(dir, "ttyUSB*"),
		filepath.Join(dir, "ttyACM*"),
	})
	want := []string{
		filepath.Join(dir, "ttyACM0"),
		filepath.Join(dir, "ttyACM3"),
		filepath.Join(dir, "ttyUSB0"),
		filepath.Join(dir, "ttyUSB1"),
	}
	if len(ports) != len(want) {
		t.Fatalf("got %v, want %v", ports, want)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("got %v, want %v", ports, want)
		}
	}

	// ttyS* and non-matching names must not appear.
	for _, p := range ports {
		if filepath.Base(p) == "ttyS0" {
			t.Errorf("ttyS0 should not be matched")
		}
	}

	if got := globPorts([]string{filepath.Join(dir, "nothing*")}); len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestMergePortOption(t *testing.T) {
	base := []string{"/dev/ttyUSB0", "/dev/ttyUSB1"}

	got := mergePortOption(base, "")
	if len(got) != 2 {
		t.Errorf("empty current should not change options: %v", got)
	}
	got = mergePortOption(base, "/dev/ttyUSB1")
	if len(got) != 2 {
		t.Errorf("duplicate current should not be added: %v", got)
	}
	got = mergePortOption(base, "/dev/pts/7")
	if len(got) != 3 || got[0] != "/dev/pts/7" {
		t.Errorf("unknown current should be prepended: %v", got)
	}
}
