package launch

import (
	"fmt"
	"strings"
	"time"
)

// readyPollInterval is how often a pane's output is checked for the agent's
// ready marker. It is a variable so tests can shorten it.
var readyPollInterval = 500 * time.Millisecond

// shellQuote wraps s in single quotes for a POSIX shell, escaping an embedded
// single quote as '\”. The result is always one literal word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// startCommand is the shell command line that starts agent a, selecting model
// when it is not empty.
func startCommand(a Agent, model string) string {
	if model == "" {
		return a.TmuxCommand
	}
	return a.TmuxCommand + " " + a.ModelFlag + " " + shellQuote(model)
}

type tmuxSpace struct{ run RunFunc }

// newTmux returns the space that drives the tmux server the user is in.
func newTmux(run RunFunc) space { return tmuxSpace{run} }

func (t tmuxSpace) tmux(stdin []byte, args ...string) ([]byte, error) {
	return t.run("", stdin, "tmux", args...)
}

func (t tmuxSpace) open(r Request) (string, error) {
	var args []string
	if r.Place == PlaceWindow {
		args = []string{"new-window"}
	} else {
		flag := "-h"
		if r.Split == SplitVertical {
			flag = "-v"
		}
		args = []string{"split-window", flag}
	}
	args = append(args, "-P", "-F", "#{pane_id}", "-c", r.Dir)
	out, err := t.tmux(nil, args...)
	if err != nil {
		return "", err
	}
	pane := strings.TrimSpace(string(out))
	if pane == "" {
		return "", fmt.Errorf("tmux did not report the new pane")
	}
	return pane, nil
}

// start types the start command into the pane. The command is quoted by
// startCommand, so nothing in the model name is run by the shell.
func (t tmuxSpace) start(pane string, r Request) error {
	if _, err := t.tmux(nil, "send-keys", "-t", pane, "-l", startCommand(r.Agent, r.Model)); err != nil {
		return err
	}
	_, err := t.tmux(nil, "send-keys", "-t", pane, "Enter")
	return err
}

// ready polls the pane's output until it contains the agent's ready marker.
// The pane is checked at least once.
func (t tmuxSpace) ready(pane string, r Request, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := t.tmux(nil, "capture-pane", "-p", "-t", pane)
		if err != nil {
			return err
		}
		if strings.Contains(string(out), r.Agent.ReadyMarker) {
			return nil
		}
		if !time.Now().Add(readyPollInterval).Before(deadline) {
			return fmt.Errorf("%s did not show %q within %s", r.Agent.Label, r.Agent.ReadyMarker, timeout)
		}
		time.Sleep(readyPollInterval)
	}
}

// paste sends text to the pane as a bracketed paste through a named tmux
// buffer, then presses Enter. The text never goes through the shell.
func (t tmuxSpace) paste(pane, text string) error {
	buf := "gh9s-" + strings.TrimPrefix(pane, "%")
	if _, err := t.tmux([]byte(text), "load-buffer", "-b", buf, "-"); err != nil {
		return err
	}
	if _, err := t.tmux(nil, "paste-buffer", "-p", "-d", "-b", buf, "-t", pane); err != nil {
		return err
	}
	_, err := t.tmux(nil, "send-keys", "-t", pane, "Enter")
	return err
}
