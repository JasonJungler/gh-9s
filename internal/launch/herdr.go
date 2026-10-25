package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// herdrStartTimeout is how long `herdr agent start` waits for the agent to be
// ready, in milliseconds. agent start returns only once the agent is ready, so
// this is the whole readiness wait.
const herdrStartTimeout = "30000"

// herdrPaneIDPattern matches the pane ids herdr prints: workspace "w1", pane
// "p2", joined as "w1:p2".
var herdrPaneIDPattern = regexp.MustCompile(`^w[0-9]+:p[0-9]+$`)

// herdrNamePattern is what herdr accepts as an agent name.
var herdrNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type herdrSpace struct {
	run    RunFunc
	getenv func(string) string
}

// newHerdr returns the space that drives the herdr session the user is in. New
// panes and tabs are made relative to $HERDR_PANE_ID, which herdr sets in every
// pane it manages; outside a herdr pane it refuses to open anything, because
// herdr would otherwise use whatever pane the UI has focused.
func newHerdr(run RunFunc, getenv func(string) string) space {
	return &herdrSpace{run: run, getenv: getenv}
}

func (h *herdrSpace) herdr(args ...string) ([]byte, error) {
	return h.run("", nil, "herdr", args...)
}

// herdrAgentName is the name given to the agent started in pane. Pane ids are
// never reused within a session, so the name is unique, and it matches
// herdrNamePattern ("w1:p2" gives "gh9s-w1-p2").
func herdrAgentName(pane string) string {
	return "gh9s-" + strings.ReplaceAll(pane, ":", "-")
}

// herdrPaneInfo is the part of herdr's JSON replies that holds a pane id.
type herdrPaneInfo struct {
	PaneID string `json:"pane_id"`
}

// parseHerdrPaneID extracts the new pane's id from the JSON that `herdr pane
// split` or `herdr tab create` printed. field is "pane" for a split and
// "root_pane" for a tab. It returns "" unless the id looks like a herdr pane
// id.
//
// Observed with herdr 0.8.2 (one line of JSON each, here shortened):
//
//	$ herdr pane split --pane w1:p1 --direction right --cwd /work
//	{"id":"cli:pane:split","result":{"pane":{"cwd":"/work","pane_id":"w1:p2",
//	  "tab_id":"w1:t1","workspace_id":"w1",...},"type":"pane_info"}}
//
//	$ herdr tab create --cwd /work --label x
//	{"id":"cli:tab:create","result":{"root_pane":{"cwd":"/work","pane_id":"w1:p3",
//	  "tab_id":"w1:t2","workspace_id":"w1",...},"tab":{"label":"x",
//	  "tab_id":"w1:t2",...},"type":"tab_created"}}
//
// A failure prints {"error":{"code":"pane_not_found",...}} on stderr and exits
// 1, so it never reaches this parser. Ids are w<N> (workspace), w<N>:t<N>
// (tab) and w<N>:p<N> (pane).
//
// Other facts pinned in the same run: `herdr agent start <name> --kind claude
// --pane w1:p5 --timeout 30000 -- --model haiku` printed {"result":{"agent":
// {"name":...,"pane_id":"w1:p5","interactive_ready":true,...},"argv":["claude",
// "--model","haiku"],"type":"agent_started"}} once Claude was up. In a folder
// Claude does not trust yet it exited 1 with {"error":{"code":"agent_not_ready",
// "message":"agent <name> is blocked during startup and is not ready for
// prompts"}}. `herdr agent prompt <TARGET> <TEXT>` accepts either the agent
// name or the pane id as TARGET (both were tried) and printed
// {"result":{"agent":{...},"type":"agent_prompted"}}. `herdr pane split
// --current` without $HERDR_PANE_ID exits 2 with "--current requires
// HERDR_PANE_ID", and omitting the target uses the UI-focused pane, which is
// why open requires $HERDR_PANE_ID.
func parseHerdrPaneID(out []byte, field string) string {
	var reply struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return ""
	}
	raw, ok := reply.Result[field]
	if !ok {
		return ""
	}
	var info herdrPaneInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ""
	}
	if !herdrPaneIDPattern.MatchString(info.PaneID) {
		return ""
	}
	return info.PaneID
}

// herdrQuote shortens herdr's output for an error message.
func herdrQuote(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return fmt.Sprintf("%q", s)
}

// open splits the pane herdr says the user is in, or creates a tab in its
// workspace. The tab is labelled "issue-<n>" so the label never starts with a
// dash.
func (h *herdrSpace) open(r Request) (string, error) {
	herdrPane := h.getenv("HERDR_PANE_ID")
	if herdrPane == "" {
		return "", errors.New("not in a herdr pane (HERDR_PANE_ID is not set)")
	}
	var args []string
	field := "pane"
	if r.Place == PlaceWindow {
		field = "root_pane"
		args = []string{"tab", "create"}
		if ws := h.getenv("HERDR_WORKSPACE_ID"); ws != "" {
			args = append(args, "--workspace", ws)
		}
		label := "agent"
		if r.Issue > 0 {
			label = fmt.Sprintf("issue-%d", r.Issue)
		}
		args = append(args, "--cwd", r.Dir, "--label", label)
	} else {
		direction := "right"
		if r.Split == SplitVertical {
			direction = "down"
		}
		args = []string{"pane", "split", "--pane", herdrPane, "--direction", direction, "--cwd", r.Dir}
	}
	out, err := h.herdr(args...)
	if err != nil {
		return "", err
	}
	pane := parseHerdrPaneID(out, field)
	if pane == "" {
		return "", fmt.Errorf("herdr printed %s instead of a pane id", herdrQuote(out))
	}
	return pane, nil
}

// check refuses, before a pane is opened, a text that paste would refuse, so nothing is left open for a
// launch that cannot succeed: a paste-only text with a control character, and (submit) a prompt that starts
// with "-". `herdr agent prompt <pane> <text>` is given the text as a bare argument (its help shows no "--"),
// so herdr could read such a prompt as an option. Paste-only text starts with ESC and is not affected.
func (h *herdrSpace) check(r Request) error {
	if r.PasteOnly && !pasteSafe(r.Context) {
		return errPasteUnsafe
	}
	if !r.PasteOnly && strings.HasPrefix(r.Context, "-") {
		return promptDashError("herdr")
	}
	return nil
}

// start runs the agent in pane. herdr waits until the agent is ready before
// it returns. The model is passed as its own argument after "--", so no shell
// ever sees it.
func (h *herdrSpace) start(pane string, r Request) error {
	if hasControl(r.Model) {
		return errors.New("model contains a control character")
	}
	args := []string{"agent", "start", herdrAgentName(pane), "--kind", r.Agent.HerdrKind, "--pane", pane, "--timeout", herdrStartTimeout}
	if r.Model != "" {
		if r.Agent.ModelFlag == "" {
			return fmt.Errorf("%s has no model flag", r.Agent.Label)
		}
		args = append(args, "--", r.Agent.ModelFlag, r.Model)
	}
	_, err := h.herdr(args...)
	return err
}

// ready does nothing: `herdr agent start` has already waited for the agent.
func (h *herdrSpace) ready(pane string, r Request, timeout time.Duration) error {
	return nil
}

// Bracketed paste markers: a terminal program that turned on bracketed paste reads what lies between them
// as pasted text, so a newline in it is a newline in the text and not the Enter key.
const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// errPasteUnsafe is why a text is refused as a paste.
var errPasteUnsafe = errors.New("the text contains a control character or invalid UTF-8, so it cannot be placed as a paste")

// pasteSafe reports whether text can sit inside a bracketed paste: valid UTF-8 (a raw 0x9b byte would be a
// C1 control character to a terminal) and no control character other than newline and tab, so it can
// neither end the paste (ESC[201~) nor press a key. BuildContext already removes these; this is the check
// at the point of sending.
func pasteSafe(text string) bool {
	if !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r <= 0x9f) {
			return false
		}
	}
	return true
}

// paste gives text to the agent in pane. With submit, `herdr agent prompt` sends it as a paste followed by
// Enter. herdr has no way to prompt without Enter, so otherwise `herdr pane send-text` is used.
//
// Observed with herdr 0.8.2 in a pane whose program echoed its raw input (stty raw -echo; cat > file),
// including after the program turned on bracketed paste (ESC[?2004h): send-text writes the text byte for
// byte, adds no newline or Enter, does not make it a bracketed paste, and passes ESC through. A newline in
// it therefore arrives as a bare LF, which an agent TUI may take for Enter. So the text is wrapped in the
// bracketed paste markers here, after it has been checked to hold no control character that could end the
// paste early. (`herdr pane run` would wrap it, but presses Enter.) The wrapped text is one argument, which
// also starts with ESC and so can never read as an option. An agent that did not turn on bracketed paste
// would show the markers as characters or submit at a newline; Claude Code and Devin are expected to turn
// it on, but that is not verified.
func (h *herdrSpace) paste(pane, text string, submit bool) error {
	if submit {
		_, err := h.herdr("agent", "prompt", pane, text)
		return err
	}
	if !pasteSafe(text) {
		return errPasteUnsafe
	}
	_, err := h.herdr("pane", "send-text", pane, bracketedPasteStart+text+bracketedPasteEnd)
	return err
}
