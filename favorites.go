package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// reservedCommands are the names the ':' command bar already understands;
// aliases may not shadow them.
var reservedCommands = []string{
	"runs", "run", "r", "workflows", "workflow", "wf", "w", "pulls", "pull",
	"prs", "pr", "cpr", "checks", "fav", "q", "quit", "exit",
}

type favorite struct {
	Host     string `json:"host"`
	Repo     string `json:"repo"`     // owner/name
	Workflow string `json:"workflow"` // file base name, e.g. deploy.yml
	Alias    string `json:"alias,omitempty"`
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validate keeps hostile or garbled values from a hand-edited (or synced) config file
// out of command lines and API paths: names are plain tokens that cannot start with "-".
func (f favorite) validate() error {
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

func (f favorite) key() string { return f.Host + "/" + f.Repo + "/" + f.Workflow }

func (f favorite) target() repoRef {
	owner, name, _ := strings.Cut(f.Repo, "/")
	return repoRef{Host: f.Host, Owner: owner, Name: name}
}

type favStore struct {
	path  string
	items []favorite
}

func favsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "favorites.json")
}

// loadFavs reads the store. A missing or empty file is an empty store; an
// unparseable file is an error and is never rewritten.
func loadFavs(path string) (*favStore, error) {
	s := &favStore{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(b))) == 0) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.items); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, it := range s.items {
		if err := it.validate(); err != nil {
			return nil, fmt.Errorf("%s: entry %d: %w", path, i+1, err)
		}
	}
	return s, nil
}

func (s *favStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *favStore) index(f favorite) int {
	for i, it := range s.items {
		if it.key() == f.key() {
			return i
		}
	}
	return -1
}

func (s *favStore) toggle(f favorite) (added bool) {
	if i := s.index(f); i >= 0 {
		s.items = append(s.items[:i], s.items[i+1:]...)
		return false
	}
	f.Alias = ""
	s.items = append(s.items, f)
	return true
}

func (s *favStore) setAlias(f favorite, alias string) error {
	i := s.index(f)
	if i < 0 {
		return errors.New("not a favorite")
	}
	if alias == "" || strings.ContainsAny(alias, " \t") {
		return errors.New("alias must be non-empty and contain no spaces")
	}
	for _, r := range reservedCommands {
		if strings.EqualFold(r, alias) {
			return fmt.Errorf("%q is a built-in command", alias)
		}
	}
	for j, it := range s.items {
		if j != i && strings.EqualFold(it.Alias, alias) {
			return fmt.Errorf("alias %q already used", alias)
		}
	}
	s.items[i].Alias = alias
	return nil
}

func hostOrDefault(h string) string {
	if h == "" {
		return "github.com"
	}
	return h
}

func (s *favStore) forRepo(r repoRef) []favorite {
	var out []favorite
	for _, it := range s.items {
		if it.Repo == r.Owner+"/"+r.Name && hostOrDefault(it.Host) == hostOrDefault(r.Host) {
			out = append(out, it)
		}
	}
	return out
}

func (s *favStore) byAlias(a string) (favorite, bool) {
	for _, it := range s.items {
		if it.Alias != "" && strings.EqualFold(it.Alias, a) {
			return it, true
		}
	}
	return favorite{}, false
}
