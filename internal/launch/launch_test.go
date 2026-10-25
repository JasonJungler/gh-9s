package launch

import (
	"errors"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectPriority(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Target
	}{
		{"orca beats tmux", map[string]string{"ORCA_WORKTREE_ID": "w1", "TMUX": "/tmp/tmux"}, TargetOrca},
		{"herdr pane id beats tmux", map[string]string{"HERDR_PANE_ID": "p1", "TMUX": "/tmp/tmux"}, TargetHerdr},
		{"herdr env", map[string]string{"HERDR_ENV": "1"}, TargetHerdr},
		{"tmux", map[string]string{"TMUX": "/tmp/tmux"}, TargetTmux},
	}
	for _, c := range cases {
		got, err := Detect(envOf(c.env), "")
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestDetectOverride(t *testing.T) {
	env := envOf(map[string]string{"ORCA_WORKTREE_ID": "w1", "TMUX": "/tmp/tmux"})
	if got, err := Detect(env, "tmux"); err != nil || got != TargetTmux {
		t.Errorf("override tmux: got %q, %v", got, err)
	}
	for _, o := range []string{"auto", ""} {
		if got, err := Detect(env, o); err != nil || got != TargetOrca {
			t.Errorf("override %q: got %q, %v; want orca", o, got, err)
		}
	}
}

func TestDetectNone(t *testing.T) {
	if _, err := Detect(envOf(nil), ""); !errors.Is(err, ErrNoEnvironment) {
		t.Errorf("err = %v, want ErrNoEnvironment", err)
	}
}
