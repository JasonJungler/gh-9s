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
	{Key: "delete_branch_after_merge", Label: "Delete branch after merge", Desc: "the answer to \"delete the branch?\" in the merge dialog starts as this",
		Kind: Bool, Default: false},
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
