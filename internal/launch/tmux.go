package launch

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// readyPollInterval is how often a pane is checked for the agent's ready
// marker. It is a variable so tests can shorten it.
var readyPollInterval = 500 * time.Millisecond

// settleTimeout caps how long start waits for a new pane's screen to stop
// changing before it takes the baseline. It is a variable so tests can shorten
// it.
var settleTimeout = 3 * time.Second

// shellQuote wraps s in single quotes for a POSIX shell. An embedded single
// quote is written as quote, backslash, quote, quote (close the string, add an
// escaped quote, reopen it). The result is always one literal word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// safeModelPattern is every model name that may be typed into a pane: it starts with a letter or digit (so it
// can never read as an option) and goes on with letters, digits and . _ : / @ + [ ] - (so "sonnet[1m]",
// "claude-opus-4-5", "anthropic/claude-sonnet-4.5" and the like). Nothing a shell treats specially except
// [ ], which stay literal inside the quotes.
var safeModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\[\]-]*$`)

// startCommand is the shell command line that starts agent a, selecting model
// when it is not empty.
func startCommand(a Agent, model string) string {
	if model == "" {
		return a.TmuxCommand
	}
	return a.TmuxCommand + " " + a.ModelFlag + " " + shellQuote(model)
}

// paneIDPattern matches the pane id tmux prints for -F #{pane_id}.
var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

// shells are the pane commands that mean the agent is not running (yet).
var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true,
	"ksh": true, "tcsh": true, "nu": true, "pwsh": true,
}

// hasControl reports whether s has a character that must never be typed into
// a terminal: anything below 0x20, or DEL.
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

type tmuxSpace struct {
	run    RunFunc
	getenv func(string) string

	mu        sync.Mutex
	baselines map[string]int // pane id -> ready-marker count before the agent started
}

// newTmux returns the space that drives the tmux server the user is in. New
// panes and windows are created relative to $TMUX_PANE when it is set.
func newTmux(run RunFunc, getenv func(string) string) space {
	return &tmuxSpace{run: run, getenv: getenv, baselines: map[string]int{}}
}

func (t *tmuxSpace) tmux(stdin []byte, args ...string) ([]byte, error) {
	return t.run("", stdin, "tmux", args...)
}

// sessionIDPattern matches the session id tmux prints for #{session_id}.
var sessionIDPattern = regexp.MustCompile(`^\$[0-9]+$`)

// open creates the pane or window. With $TMUX_PANE set, a split targets that
// pane and a new window goes into that pane's session (new-window cannot take
// a pane as its target, only a session or window).
func (t *tmuxSpace) open(r Request) (string, error) {
	tmuxPane := t.getenv("TMUX_PANE")
	var args []string
	if r.Place == PlaceWindow {
		args = []string{"new-window", "-P", "-F", "#{pane_id}"}
		if tmuxPane != "" {
			b, err := t.tmux(nil, "display-message", "-p", "-t", tmuxPane, "#{session_id}")
			if err != nil {
				return "", err
			}
			session := strings.TrimSpace(string(b))
			if !sessionIDPattern.MatchString(session) {
				return "", fmt.Errorf("tmux printed %q instead of a session id", session)
			}
			args = append(args, "-t", session)
		}
	} else {
		flag := "-h"
		if r.Split == SplitVertical {
			flag = "-v"
		}
		args = []string{"split-window", flag, "-P", "-F", "#{pane_id}"}
		if tmuxPane != "" {
			args = append(args, "-t", tmuxPane)
		}
	}
	args = append(args, "-c", r.Dir)
	out, err := t.tmux(nil, args...)
	if err != nil {
		return "", err
	}
	pane := strings.TrimSpace(string(out))
	if !paneIDPattern.MatchString(pane) {
		return "", fmt.Errorf("tmux printed %q instead of a pane id", pane)
	}
	return pane, nil
}

// settle waits until the pane shows some text that is the same on two
// consecutive checks (the new shell has drawn its prompt) and returns that
// screen. If that does not happen within settleTimeout it fails, so nothing is
// typed into a pane whose state is unknown. A baseline taken before the shell has drawn
// its prompt would let the prompt itself look like a new marker.
func (t *tmuxSpace) settle(pane string) (string, error) {
	deadline := time.Now().Add(settleTimeout)
	prev := ""
	for {
		b, err := t.tmux(nil, "capture-pane", "-p", "-t", pane)
		if err != nil {
			return "", err
		}
		out := string(b)
		if strings.TrimSpace(out) != "" && out == prev {
			return out, nil
		}
		prev = out
		if !time.Now().Add(readyPollInterval).Before(deadline) {
			return "", fmt.Errorf("pane did not settle within %s", settleTimeout)
		}
		time.Sleep(readyPollInterval)
	}
}

// check validates what will be typed into the pane. runSequence calls it before open, so a rejected model
// leaves no empty pane or window behind; start calls it again as the last guard before anything is typed.
func (t *tmuxSpace) check(r Request) error {
	if hasControl(r.Model) || hasControl(r.Agent.TmuxCommand) {
		return errors.New("model or command contains a control character")
	}
	if r.Model != "" && !safeModelPattern.MatchString(r.Model) {
		return fmt.Errorf("model %q must start with a letter or digit and have only letters, digits and . _ : / @ + [ ] -, so it cannot be typed into a shell safely", r.Model)
	}
	return nil
}

// start records how often the ready marker is already on screen (the shell's
// prompt may contain it), then types the start command into the pane. The
// command is typed into whatever shell the pane runs, which may not be POSIX
// (fish and nu quote differently), so the model must match safeModelPattern
// before anything is typed; startCommand's quoting is a second layer. An empty
// model passes no flag.
func (t *tmuxSpace) start(pane string, r Request) error {
	if err := t.check(r); err != nil {
		return err
	}
	base := 0
	if r.Agent.ReadyMarker != "" {
		out, err := t.settle(pane)
		if err != nil {
			return err
		}
		base = strings.Count(out, r.Agent.ReadyMarker)
	}
	t.mu.Lock()
	t.baselines[pane] = base
	t.mu.Unlock()
	if _, err := t.tmux(nil, "send-keys", "-t", pane, "-l", startCommand(r.Agent, r.Model)); err != nil {
		return err
	}
	_, err := t.tmux(nil, "send-keys", "-t", pane, "Enter")
	return err
}

// ready polls the pane until the agent is up. That needs both: the ready
// marker appears more often than it did before start (so a prompt or the typed
// command cannot satisfy it), and the pane's foreground command is not a shell.
// The pane is checked at least once.
func (t *tmuxSpace) ready(pane string, r Request, timeout time.Duration) error {
	marker := r.Agent.ReadyMarker
	if marker == "" {
		return fmt.Errorf("%s has no ready marker, so readiness cannot be detected", r.Agent.Label)
	}
	t.mu.Lock()
	base := t.baselines[pane]
	t.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for {
		out, err := t.tmux(nil, "capture-pane", "-p", "-t", pane)
		if err != nil {
			return err
		}
		if strings.Count(string(out), marker) > base {
			cmd, err := t.tmux(nil, "display-message", "-p", "-t", pane, "#{pane_current_command}")
			if err != nil {
				return err
			}
			if !shells[strings.TrimPrefix(strings.TrimSpace(string(cmd)), "-")] {
				t.mu.Lock()
				delete(t.baselines, pane)
				t.mu.Unlock()
				return nil
			}
		}
		if !time.Now().Add(readyPollInterval).Before(deadline) {
			return fmt.Errorf("%s did not show %q within %s", r.Agent.Label, marker, timeout)
		}
		time.Sleep(readyPollInterval)
	}
}

// paste sends text to the pane as a bracketed paste through a named tmux
// buffer, then presses Enter. The text never goes through the shell.
func (t *tmuxSpace) paste(pane, text string) error {
	buf := "gh9s-" + strings.TrimPrefix(pane, "%")
	if _, err := t.tmux([]byte(text), "load-buffer", "-b", buf, "-"); err != nil {
		return err
	}
	if _, err := t.tmux(nil, "paste-buffer", "-p", "-d", "-b", buf, "-t", pane); err != nil {
		t.tmux(nil, "delete-buffer", "-b", buf) // best effort: do not leave the text in tmux
		return err
	}
	_, err := t.tmux(nil, "send-keys", "-t", pane, "Enter")
	return err
}
