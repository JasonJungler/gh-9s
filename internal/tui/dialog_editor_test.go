package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorCommandRunsTheEditorOnAFileHoldingTheText(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake editor.sh") // a path with a space, as a real one might have
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s+edited' \"$(cat \"$1\")\" > \"$1\"\n"), 0o755)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "'"+script+"'")
	cmd, path, err := editorCommand("draft")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if strings.Contains(cmd.Args[2], path) {
		t.Errorf("the file name must be an argument, not part of the script: %q", cmd.Args[2])
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "draft+edited" {
		t.Errorf("file = %q; want the editor to have seen and changed the text", b)
	}
}

func TestEditorFallsBackToViWhenNothingIsSet(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	cmd, path, err := editorCommand("")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	if !strings.HasPrefix(cmd.Args[2], "vi ") {
		t.Errorf("script = %q; want vi", cmd.Args[2])
	}
}
