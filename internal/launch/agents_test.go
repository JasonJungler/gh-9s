package launch

import (
	"errors"
	"reflect"
	"testing"
)

func ids(ms []Model) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func TestLookupAgent(t *testing.T) {
	for _, id := range []string{"claude", "devin"} {
		a, ok := LookupAgent(id)
		if !ok || a.ID != id || a.OrcaID != id || a.HerdrKind != id || a.TmuxCommand != id || a.ModelFlag != "--model" {
			t.Errorf("LookupAgent(%q) = %+v, %v", id, a, ok)
		}
	}
	if _, ok := LookupAgent("nope"); ok {
		t.Error("LookupAgent(nope) should not be found")
	}
}

func TestClaudeModels(t *testing.T) {
	a, _ := LookupAgent("claude")
	run := func(string, []byte, string, ...string) ([]byte, error) {
		t.Error("run must not be called for claude")
		return nil, nil
	}
	got := ListModels(a, run)
	want := []string{"", "best", "fable", "opus", "sonnet", "haiku", "opusplan", "sonnet[1m]", "opus[1m]"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Errorf("ids = %q, want %q", ids(got), want)
	}
}

func TestDevinModelsLive(t *testing.T) {
	a, _ := LookupAgent("devin")
	tests := map[string]string{
		"strings":       `["opus","swe"]`,
		"objects id":    `[{"id":"opus"},{"id":"swe"}]`,
		"objects name":  `[{"name":"opus"},{"name":"swe"}]`,
		"models strs":   `{"models":["opus","swe"]}`,
		"models object": `{"models":[{"id":"opus","description":"x"},{"name":"swe"}]}`,
	}
	for name, out := range tests {
		t.Run(name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			run := func(_ string, _ []byte, n string, args ...string) ([]byte, error) {
				gotName, gotArgs = n, args
				return []byte(out), nil
			}
			got := ListModels(a, run)
			if want := []string{"", "opus", "swe"}; !reflect.DeepEqual(ids(got), want) {
				t.Errorf("ids = %q, want %q", ids(got), want)
			}
			if gotName != "devin" || !reflect.DeepEqual(gotArgs, []string{"models", "list", "--format", "json"}) {
				t.Errorf("ran %q %q", gotName, gotArgs)
			}
		})
	}
}

func TestDevinModelsFallback(t *testing.T) {
	a, _ := LookupAgent("devin")
	want := []string{"", "opus", "sonnet", "swe", "codex", "gemini"}
	cases := map[string]RunFunc{
		"error": func(string, []byte, string, ...string) ([]byte, error) {
			return nil, errors.New("boom")
		},
		"empty": func(string, []byte, string, ...string) ([]byte, error) { return nil, nil },
		"garbage": func(string, []byte, string, ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
		"wrong shape": func(string, []byte, string, ...string) ([]byte, error) {
			return []byte(`{"foo":1}`), nil
		},
		"empty list": func(string, []byte, string, ...string) ([]byte, error) {
			return []byte(`[]`), nil
		},
		"unusable items": func(string, []byte, string, ...string) ([]byte, error) {
			return []byte(`[1,2]`), nil
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ListModels(a, run); !reflect.DeepEqual(ids(got), want) {
				t.Errorf("ids = %q, want %q", ids(got), want)
			}
		})
	}
}

func TestExecRun(t *testing.T) {
	out, err := ExecRun("", []byte("hi"), "cat")
	if err != nil || string(out) != "hi" {
		t.Errorf("cat: %q, %v", out, err)
	}
	_, err = ExecRun("", nil, "sh", "-c", "echo '  bad thing  ' >&2; exit 3")
	if err == nil || err.Error() != "bad thing" {
		t.Errorf("err = %v, want %q", err, "bad thing")
	}
}
