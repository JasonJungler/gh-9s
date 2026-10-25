package launch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-9s", "launch.json")
	p, err := LoadPrefs(path)
	if err != nil {
		t.Fatal(err)
	}
	p.SetModel("claude", "opus")
	p.SetModel("devin", "swe-1.5")
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	q, err := LoadPrefs(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Model("claude"); got != "opus" {
		t.Errorf("claude model = %q, want opus", got)
	}
	if got := q.Model("devin"); got != "swe-1.5" {
		t.Errorf("devin model = %q, want swe-1.5", got)
	}
	if got := q.Model("other"); got != "" {
		t.Errorf("unknown agent model = %q, want empty", got)
	}
}

func TestPrefsMissingFileIsEmpty(t *testing.T) {
	p, err := LoadPrefs(filepath.Join(t.TempDir(), "nope", "launch.json"))
	if err != nil {
		t.Fatalf("a missing file is not an error, got %v", err)
	}
	if p == nil || p.Model("claude") != "" {
		t.Errorf("want empty prefs, got %+v", p)
	}
}

func TestPrefsEmptyFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.json")
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPrefs(path)
	if err != nil || p.Model("claude") != "" {
		t.Errorf("empty file: prefs %+v, err %v", p, err)
	}
}

func TestPrefsCorruptFileKeepsDefaultsAndReports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.json")
	const bad = `{"models": {"claude": "opus"` // truncated
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPrefs(path)
	if err == nil {
		t.Fatal("a corrupt file must be reported")
	}
	if p == nil {
		t.Fatal("prefs must be usable even when the file is corrupt")
	}
	if got := p.Model("claude"); got != "" {
		t.Errorf("model = %q, want default", got)
	}
	p.SetModel("claude", "sonnet")
	if got := p.Model("claude"); got != "sonnet" {
		t.Errorf("a change still applies for this run, got %q", got)
	}
	_ = p.Save() // may report why it did not save; it must not touch the file
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(b) != bad {
		t.Errorf("corrupt file was overwritten: %q", b)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("a temp file was left behind")
	}
}

func TestPrefsWrongShapeIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.json")
	const bad = `["not", "an", "object"]`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPrefs(path)
	if err == nil || p == nil {
		t.Fatalf("want usable prefs and an error, got %v %v", p, err)
	}
	p.SetModel("claude", "x")
	if p.Save() == nil {
		t.Error("Save on unreadable prefs should say it did not save")
	}
	if b, _ := os.ReadFile(path); string(b) != bad {
		t.Errorf("file overwritten: %q", b)
	}
}

func TestPrefsNilIsSafe(t *testing.T) {
	var p *Prefs
	if p.Model("claude") != "" {
		t.Error("nil Model should be empty")
	}
	p.SetModel("claude", "opus")
	if err := p.Save(); err != nil {
		t.Errorf("nil Save should be a no-op, got %v", err)
	}
}

func TestPrefsEmptyModelMeansDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.json")
	p, _ := LoadPrefs(path)
	p.SetModel("claude", "opus")
	p.SetModel("claude", "  ")
	if got := p.Model("claude"); got != "" {
		t.Errorf("model = %q, want empty", got)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	q, err := LoadPrefs(path)
	if err != nil || q.Model("claude") != "" {
		t.Errorf("reloaded model = %q, err %v", q.Model("claude"), err)
	}
}
