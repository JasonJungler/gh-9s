package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// runFinished is true when a run went from some unfinished status to completed.
func runFinished(prev, cur string) bool {
	return prev != "" && prev != "completed" && cur == "completed"
}

// runOutcome words a run's conclusion for a notification.
func runOutcome(conclusion string) string {
	switch conclusion {
	case "success":
		return "succeeded"
	case "failure":
		return "failed"
	case "cancelled":
		return "was cancelled"
	case "timed_out":
		return "timed out"
	case "":
		return "finished"
	}
	return conclusion
}

// notifier is a variable so tests can observe notifications.
var notifier = notify

// notify rings the terminal bell and, where available, raises a desktop notification.
// Failures are ignored: a missing notifier must never disturb the TUI.
func notify(title, body string) {
	fmt.Fprint(os.Stdout, "\a")
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("osascript", osascriptArgs(title, body)...).Run()
	case "linux":
		if p, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command(p, title, body).Run()
		}
	}
}

func osascriptArgs(title, body string) []string {
	esc := func(s string) string {
		return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	}
	return []string{"-e", fmt.Sprintf(`display notification "%s" with title "%s"`, esc(body), esc(title))}
}
