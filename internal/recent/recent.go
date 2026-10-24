// Package recent remembers the repos the user switched to, newest first, in a personal
// config file, validating everything it reads back.
package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

const maxEntries = 20

// token is what a host, owner or repo name may look like; it cannot start with "-".
var token = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Store is the list of recently used repos.
type Store struct {
	path  string
	repos []ghcli.RepoRef
}

// DefaultPath is the per-user file next to favorites.json.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "recent.json")
}

func valid(r ghcli.RepoRef) bool {
	for _, p := range []string{r.Owner, r.Name} {
		if !token.MatchString(p) {
			return false
		}
	}
	return r.Host == "" || token.MatchString(r.Host)
}

// Load reads the store. A missing or empty file is an empty store; an unparseable file or
// an invalid entry is an error and the file is left untouched.
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && strings.TrimSpace(string(b)) == "") {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var raw []string
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, x := range raw {
		r, err := ghcli.ParseRepoRef(x)
		if err != nil || !valid(r) {
			return nil, fmt.Errorf("%s: entry %d: invalid repo %q", path, i+1, x)
		}
		s.repos = append(s.repos, r)
	}
	return s, nil
}

// Add records r as the most recently used repo. The zero value (the current directory) is ignored.
func (s *Store) Add(r ghcli.RepoRef) {
	if r == (ghcli.RepoRef{}) {
		return
	}
	next := []ghcli.RepoRef{r}
	for _, x := range s.repos {
		if x != r {
			next = append(next, x)
		}
	}
	if len(next) > maxEntries {
		next = next[:maxEntries]
	}
	s.repos = next
}

// List returns the repos, newest first.
func (s *Store) List() []ghcli.RepoRef { return append([]ghcli.RepoRef(nil), s.repos...) }

// Save writes the store atomically (temp file, then rename).
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw := make([]string, len(s.repos))
	for i, r := range s.repos {
		raw[i] = r.String()
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
