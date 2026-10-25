package launch

import (
	"encoding/json"
	"time"
)

// Agent describes a coding agent the launcher knows how to start.
type Agent struct {
	ID    string // identifier used in the config and the menu
	Label string // name shown to the user

	OrcaID      string // agent id for orca's CLI
	HerdrKind   string // agent kind for herdr
	TmuxCommand string // binary started in a tmux pane or window
	ModelFlag   string // flag that selects the model

	// ReadyMarker is a substring of the agent's screen that appears only once
	// it is ready to accept a prompt.
	ReadyMarker string
	// Verified reports whether ReadyMarker and the model list were confirmed
	// against the real agent.
	Verified bool
}

// Agents is the table of supported agents.
var Agents = []Agent{
	{
		ID: "claude", Label: "Claude Code",
		OrcaID: "claude", HerdrKind: "claude", TmuxCommand: "claude", ModelFlag: "--model",
		// Observed with Claude Code 2.1.310 in a throwaway tmux server: once
		// started it draws a banner whose first line is "Claude Code v2.1.310"
		// together with the input box (a "❯ Try ..." placeholder whose text
		// varies, so it is not used). In a folder that is not yet trusted it
		// first shows a "Quick safety check" screen with a "❯ No, exit" menu;
		// that screen has no "Claude Code v" text, so it does not match.
		ReadyMarker: "Claude Code v",
		Verified:    true,
	},
	{
		ID: "devin", Label: "Devin",
		OrcaID: "devin", HerdrKind: "devin", TmuxCommand: "devin", ModelFlag: "--model",
		// UNVERIFIED: devin is not installed where this was written, so neither
		// the ready marker nor the JSON shape printed by `devin models list
		// --format json` is confirmed. docs.devin.ai/cli/reference/commands
		// says the command exists and that --format json is "for scripts", but
		// not what the JSON looks like; parseDevinModels is a tolerant guess.
		// The marker is the prompt glyph most terminal agents draw.
		ReadyMarker: "❯",
		Verified:    false,
	},
}

// LookupAgent returns the agent with the given ID.
func LookupAgent(id string) (Agent, bool) {
	for _, a := range Agents {
		if a.ID == id {
			return a, true
		}
	}
	return Agent{}, false
}

// Model is a selectable model. An empty ID means the agent's default.
type Model struct{ ID, Hint string }

var defaultModel = Model{ID: "", Hint: "default"}

var claudeModels = []Model{
	{"best", "most capable"},
	{"fable", ""},
	{"opus", ""},
	{"sonnet", ""},
	{"haiku", "fastest"},
	{"opusplan", "opus to plan, sonnet to build"},
	{"sonnet[1m]", "1M context"},
	{"opus[1m]", "1M context"},
}

var devinFallbackModels = []string{"opus", "sonnet", "swe", "codex", "gemini"}

// devinListTimeout caps the `devin models list` call.
const devinListTimeout = 5 * time.Second

// ListModels returns the models offered for a, starting with the default.
// Claude's list is fixed. Devin's comes from `devin models list --format json`
// and falls back to a built-in list on any failure.
func ListModels(a Agent, run RunFunc) []Model {
	models := []Model{defaultModel}
	switch a.ID {
	case "claude":
		return append(models, claudeModels...)
	case "devin":
		for _, id := range devinModelIDs(run) {
			models = append(models, Model{ID: id})
		}
	}
	return models
}

// devinModelIDs asks devin for its models, or returns the fallback list.
func devinModelIDs(run RunFunc) []string {
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := run("", nil, "devin", "models", "list", "--format", "json")
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		if r.err == nil {
			if ids := parseDevinModels(r.out); len(ids) > 0 {
				return ids
			}
		}
	case <-time.After(devinListTimeout):
	}
	return devinFallbackModels
}

// parseDevinModels extracts model IDs from the output of `devin models list
// --format json`. The real shape is unconfirmed (see the devin row of Agents),
// so it accepts a JSON array, or an object with a "models" array, whose items
// are either strings or objects with a string "id" (or, failing that, "name")
// field. Anything else yields nil.
func parseDevinModels(out []byte) []string {
	var raw json.RawMessage = out
	var wrapped struct {
		Models json.RawMessage `json:"models"`
	}
	if json.Unmarshal(out, &wrapped) == nil && wrapped.Models != nil {
		raw = wrapped.Models
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var ids []string
	for _, it := range items {
		var s string
		if json.Unmarshal(it, &s) == nil {
			if s != "" {
				ids = append(ids, s)
			}
			continue
		}
		var o struct{ ID, Name string }
		if json.Unmarshal(it, &o) == nil {
			if o.ID != "" {
				ids = append(ids, o.ID)
			} else if o.Name != "" {
				ids = append(ids, o.Name)
			}
		}
	}
	return ids
}
