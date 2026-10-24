package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsMissingFileIsTheZeroValue(t *testing.T) {
	if p := loadPrefs(filepath.Join(t.TempDir(), "nope.json")); p.DeleteBranchAfterMerge {
		t.Errorf("prefs = %+v; want the zero value", p)
	}
	if p := loadPrefs(""); p.DeleteBranchAfterMerge {
		t.Errorf("an empty path must be harmless: %+v", p)
	}
}

func TestPrefsRoundTripCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "prefs.json")
	if err := (prefs{DeleteBranchAfterMerge: true}).save(path); err != nil {
		t.Fatal(err)
	}
	if p := loadPrefs(path); !p.DeleteBranchAfterMerge {
		t.Errorf("prefs = %+v after a round trip", p)
	}
}

func TestPrefsCorruptFileIsIgnoredNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	os.WriteFile(path, []byte("{nope"), 0o644)
	if p := loadPrefs(path); p.DeleteBranchAfterMerge {
		t.Errorf("prefs = %+v; a corrupt file should read as the zero value", p)
	}
	if b, _ := os.ReadFile(path); string(b) != "{nope" {
		t.Errorf("loading must not rewrite the file: %q", b)
	}
}
