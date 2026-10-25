package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// shortWriter fails like a full disk: the write stores part of the text and reports an error.
type shortWriter struct {
	got      strings.Builder
	closed   bool
	closeErr error
}

func (w *shortWriter) Write(p []byte) (int, error) {
	n := len(p) / 2
	w.got.Write(p[:n])
	return n, errors.New("no space left on device")
}
func (w *shortWriter) Close() error { w.closed = true; return w.closeErr }

func TestWriteAndCloseReportsAShortWriteAndStillCloses(t *testing.T) {
	w := &shortWriter{}
	err := writeAndClose(w, "the whole prompt")
	if err == nil || !strings.Contains(err.Error(), "no space left") {
		t.Errorf("err = %v; want the write error", err)
	}
	if !w.closed {
		t.Error("the file was not closed after a failed write")
	}
}

func TestWriteAndCloseReportsAFailedClose(t *testing.T) {
	ok := &okWriter{closeErr: errors.New("flush failed")}
	if err := writeAndClose(ok, "x"); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Errorf("err = %v; want the close error", err)
	}
	if err := writeAndClose(&okWriter{}, "x"); err != nil {
		t.Errorf("err = %v; want none", err)
	}
}

type okWriter struct{ closeErr error }

func (w *okWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *okWriter) Close() error                { return w.closeErr }

// A file that cannot be written must not be handed to the editor: its content would be read back as the
// edited prompt or comment. The file is removed and the failure is returned (also by editorExec).
func TestEditorCommandRefusesAFileThatCannotBeWritten(t *testing.T) {
	var path string
	old := createEditorFile
	createEditorFile = func() (*os.File, error) {
		f, err := os.CreateTemp(t.TempDir(), "gh9s-*.md")
		if err != nil {
			return nil, err
		}
		path = f.Name()
		f.Close() // writing to a closed file fails
		return f, nil
	}
	defer func() { createEditorFile = old }()

	cmd, _, err := editorCommand("the whole prompt")
	if err == nil || cmd != nil {
		t.Fatalf("cmd = %v, err = %v; want a failure and no command", cmd, err)
	}
	if _, serr := os.Stat(path); !os.IsNotExist(serr) {
		t.Errorf("the half-written file was left behind: %v", serr)
	}
	msg := editorExec("the whole prompt", func(text string, err error) tea.Msg { return editedMsg{text: text, err: err} })()
	if got, ok := msg.(editedMsg); !ok || got.err == nil || got.text != "" {
		t.Errorf("msg = %#v; want the failure and no text", msg)
	}
}
