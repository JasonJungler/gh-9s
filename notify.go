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

// runStatus summarises a run from its jobs: "" (no jobs yet), "in_progress", or "completed".
func runStatus(jobs []runJob) string {
	if len(jobs) == 0 {
		return ""
	}
	for _, j := range jobs {
		if j.Status != "completed" {
			return "in_progress"
		}
	}
	return "completed"
}

func runOutcome(jobs []runJob) string {
	for _, j := range jobs {
		if _, s := j.state(); s == stFail {
			return "failed"
		}
	}
	return "succeeded"
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
