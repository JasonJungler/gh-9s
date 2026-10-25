// Package launch sends an issue to a coding agent running in the user's
// terminal environment (Orca, herdr or tmux).
package launch

import (
	"errors"
	"fmt"
)

// Target is the environment an agent is launched in.
type Target string

// The supported launch targets.
const (
	TargetOrca  Target = "orca"
	TargetHerdr Target = "herdr"
	TargetTmux  Target = "tmux"
)

// Place says where the agent opens: a pane or a window.
type Place string

// The supported places.
const (
	PlacePane   Place = "pane"
	PlaceWindow Place = "window"
)

// Split is the direction a new pane is split in.
type Split string

// The supported split directions.
const (
	SplitHorizontal Split = "horizontal"
	SplitVertical   Split = "vertical"
)

// ErrNoEnvironment means no supported environment was detected.
var ErrNoEnvironment = errors.New("no supported environment found (orca, herdr or tmux)")

// Detect picks the launch target. A non-empty override other than "auto" wins;
// otherwise the environment decides, in the order orca, herdr, tmux.
func Detect(getenv func(string) string, override string) (Target, error) {
	switch override {
	case "", "auto":
	case string(TargetOrca), string(TargetHerdr), string(TargetTmux):
		return Target(override), nil
	default:
		return "", fmt.Errorf("unknown launch target %q", override)
	}
	switch {
	case getenv("ORCA_WORKTREE_ID") != "":
		return TargetOrca, nil
	case getenv("HERDR_ENV") != "" || getenv("HERDR_PANE_ID") != "":
		return TargetHerdr, nil
	case getenv("TMUX") != "":
		return TargetTmux, nil
	}
	return "", ErrNoEnvironment
}
