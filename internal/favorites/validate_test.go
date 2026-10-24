package favorites

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFavsRejectsHostileEntries(t *testing.T) {
	bad := []string{
		`[{"host":"github.com","repo":"a/b;rm -rf ~","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"-x/y","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"a/b","workflow":"../../user"}]`,
		`[{"host":"github.com","repo":"a/b","workflow":"--flag"}]`,
		`[{"host":"gh.com/evil","repo":"a/b","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"nope","workflow":"x.yml"}]`,
	}
	for _, src := range bad {
		p := filepath.Join(t.TempDir(), "f.json")
		os.WriteFile(p, []byte(src), 0o644)
		if _, err := Load(p); err == nil {
			t.Errorf("accepted %s", src)
		}
		if b, _ := os.ReadFile(p); string(b) != src {
			t.Errorf("file was modified for %s", src)
		}
	}
	good := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(good, []byte(`[{"host":"ghe.corp.com","repo":"acme/api-2","workflow":"deploy.yml","alias":"p"}]`), 0o644)
	if s, err := Load(good); err != nil || len(s.Items) != 1 {
		t.Errorf("valid entry rejected: %v", err)
	}
}
