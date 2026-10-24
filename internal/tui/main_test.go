package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain puts a stub "gh" first on PATH. Unit tests must never reach GitHub; with the stub, any
// call that slips past a test seam fails immediately instead of using the network.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gh9s-stub")
	if err != nil {
		panic(err)
	}
	stub := filepath.Join(dir, "gh")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho 'gh is stubbed in unit tests' >&2\nexit 1\n"), 0o755); err != nil {
		panic(err)
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
