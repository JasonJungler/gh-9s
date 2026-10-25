// Package settings is gh-9s's configuration: a registry of the settings that exist, and a store that
// reads them from a personal file and the environment, validates every value, and saves changes
// without ever overwriting a file it could not understand. Adding a setting is adding one Def.
package settings

import "strings"

// Kind is the type of a setting's value.
type Kind int

const (
	Bool Kind = iota // on/off
	Enum             // one of Options
	Int              // a whole number in [Min, Max]
)

// Def declares one setting.
type Def struct {
	Key, Label, Desc string
	Kind             Kind
	Default          any      // bool, string or int, matching Kind
	Options          []string // Enum: the allowed values, in cycling order
	Min, Max         int      // Int: the allowed range
	Env              string   // an environment variable that overrides the file ("" for none)
	EnvOptions       []string // Enum: the values the environment may give (default: all Options)
}

// Registry lists the settings in the order :settings shows them.
var Registry = []Def{
	{Key: "icons", Label: "Icons", Desc: "ask: ask on the next start; nerd: Nerd Font icons; plain: ordinary symbols",
		Kind: Enum, Default: "ask", Options: []string{"ask", "nerd", "plain"}, Env: "GH9S_ICONS", EnvOptions: []string{"nerd", "plain"}},
	{Key: "preview", Label: "Preview pane", Desc: "show the selected item in a pane on the right of the lists",
		Kind: Bool, Default: true},
	{Key: "preview_width", Label: "Preview width", Desc: "percent of the screen the preview pane takes",
		Kind: Int, Default: 40, Min: 25, Max: 60},
	{Key: "preview_min_columns", Label: "Preview minimum columns", Desc: "the narrowest terminal that shows the preview pane",
		Kind: Int, Default: 120, Min: 80, Max: 400},
	{Key: "mouse", Label: "Mouse wheel", Desc: "scroll with the wheel; while on, select text by holding Option (iTerm2, Terminal.app) or Shift (Windows Terminal, tmux)",
		Kind: Bool, Default: true},
	{Key: "delete_branch_after_merge", Label: "Delete branch after merge", Desc: "the answer to \"delete the branch?\" in the merge dialog starts as this",
		Kind: Bool, Default: false},
	{Key: "launch_target", Label: "Launch target", Desc: "where to start a coding agent for an issue; auto: the one gh-9s is running inside (orca, herdr or tmux)",
		Kind: Enum, Default: "auto", Options: []string{"auto", "orca", "herdr", "tmux"}},
	{Key: "launch_agent", Label: "Launch agent", Desc: "the coding agent to start for an issue",
		Kind: Enum, Default: "claude", Options: []string{"claude", "devin"}},
	{Key: "launch_tmux_place", Label: "Launch place", Desc: "open the agent in a pane or a window; herdr uses this too (its window is a tab)",
		Kind: Enum, Default: "pane", Options: []string{"pane", "window"}},
	{Key: "launch_split", Label: "Launch split", Desc: "which way a new pane splits; herdr uses this too",
		Kind: Enum, Default: "horizontal", Options: []string{"horizontal", "vertical"}},
	{Key: "launch_submit", Label: "Launch submit", Desc: "on: send the issue to the agent at once; off: only put it in the agent's input box, you press Enter",
		Kind: Bool, Default: true},
}

// Lookup finds a setting by key.
func Lookup(key string) (Def, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, d := range Registry {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}
