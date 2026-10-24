// Package favorites stores the user's favorite workflows and their aliases in a
// personal config file, validating everything it reads back.
package favorites

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

// ReservedCommands are the names the ':' command bar already understands;
// aliases may not shadow them.
var ReservedCommands = []string{
	"runs", "run", "r", "workflows", "workflow", "wf", "w", "pulls", "pull",
	"prs", "pr", "cpr", "checks", "fav", "favs", "favorites", "issues", "issue", "is", "inbox", "in", "settings", "set", "repo", "q", "quit", "exit",
}

// IsReserved says whether a name is a built-in command (ignoring case).
func IsReserved(name string) bool {
	for _, r := range ReservedCommands {
		if strings.EqualFold(r, name) {
			return true
		}
	}
	return false
}

// Favorite is a workflow in a specific host and repo, optionally with an alias.
type Favorite struct {
	Host     string `json:"host"`
	Repo     string `json:"repo"`     // owner/name
	Workflow string `json:"workflow"` // file base name, e.g. deploy.yml
	Alias    string `json:"alias,omitempty"`
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validate keeps hostile or garbled values from a hand-edited (or synced) config file
// out of command lines and API paths: names are plain tokens that cannot start with "-".
func (f Favorite) validate() error {
	owner, name, ok := strings.Cut(f.Repo, "/")
	switch {
	case !ok || !safeName.MatchString(owner) || !safeName.MatchString(name):
		return fmt.Errorf("invalid repo %q (want owner/name)", f.Repo)
	case f.Host != "" && !safeName.MatchString(f.Host):
		return fmt.Errorf("invalid host %q", f.Host)
	case !safeName.MatchString(f.Workflow) || f.Workflow == "." || f.Workflow == "..":
		return fmt.Errorf("invalid workflow file %q", f.Workflow)
	}
	return nil
}

// Key uniquely identifies the favorite (host, repo and workflow file).
func (f Favorite) Key() string { return f.Host + "/" + f.Repo + "/" + f.Workflow }

// Target is the favorite's repository.
func (f Favorite) Target() ghcli.RepoRef {
	owner, name, _ := strings.Cut(f.Repo, "/")
	return ghcli.RepoRef{Host: f.Host, Owner: owner, Name: name}
}

// Store is the user's list of favorites, persisted as JSON outside any repo.
type Store struct {
	path  string
	Items []Favorite
}

// DefaultPath is the per-user config file location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "favorites.json")
}

// Load reads the store. A missing or empty file is an empty store; an
// unparseable file is an error and is never rewritten.
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(b))) == 0) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.Items); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, it := range s.Items {
		if err := it.validate(); err != nil {
			return nil, fmt.Errorf("%s: entry %d: %w", path, i+1, err)
		}
	}
	return s, nil
}

// Path is the file the store reads and writes.
func (s *Store) Path() string { return s.path }

// Save writes the store atomically (temp file, then rename).
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.Items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Index returns the position of f in the store, or -1.
func (s *Store) Index(f Favorite) int {
	for i, it := range s.Items {
		if it.Key() == f.Key() {
			return i
		}
	}
	return -1
}

// Toggle adds f if absent or removes it if present, and reports whether it was added.
func (s *Store) Toggle(f Favorite) (added bool) {
	if i := s.Index(f); i >= 0 {
		s.Items = append(s.Items[:i], s.Items[i+1:]...)
		return false
	}
	f.Alias = ""
	s.Items = append(s.Items, f)
	return true
}

// SetAlias names a favorite ("" clears it). Aliases are free of spaces, unique (case-insensitive) and never a built-in command.
func (s *Store) SetAlias(f Favorite, alias string) error {
	i := s.Index(f)
	if i < 0 {
		return errors.New("not a Favorite")
	}
	if alias == "" { // clearing
		s.Items[i].Alias = ""
		return nil
	}
	if strings.ContainsAny(alias, " \t") {
		return errors.New("alias must contain no spaces")
	}
	for _, r := range ReservedCommands {
		if strings.EqualFold(r, alias) {
			return fmt.Errorf("%q is a built-in command", alias)
		}
	}
	for j, it := range s.Items {
		if j != i && strings.EqualFold(it.Alias, alias) {
			return fmt.Errorf("alias %q already used", alias)
		}
	}
	s.Items[i].Alias = alias
	return nil
}

// HostOrDefault treats an empty host as github.com.
func HostOrDefault(h string) string {
	if h == "" {
		return "github.com"
	}
	return h
}

// ForRepo lists the favorites that belong to r.
func (s *Store) ForRepo(r ghcli.RepoRef) []Favorite {
	var out []Favorite
	for _, it := range s.Items {
		if it.Repo == r.Owner+"/"+r.Name && HostOrDefault(it.Host) == HostOrDefault(r.Host) {
			out = append(out, it)
		}
	}
	return out
}

// ByAlias finds a favorite by alias, ignoring case.
func (s *Store) ByAlias(a string) (Favorite, bool) {
	for _, it := range s.Items {
		if it.Alias != "" && strings.EqualFold(it.Alias, a) {
			return it, true
		}
	}
	return Favorite{}, false
}
