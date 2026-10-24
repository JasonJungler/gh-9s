// Package drafts keeps the inline review comments a user has written but not sent, per pull request,
// in a personal file, so closing gh-9s (or losing the terminal) loses nothing. Everything read back
// is validated; a file that does not check out is an error and is left untouched.
package drafts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/MurabitoB/gh-9s/internal/safe"
)

const (
	maxPRs       = 200
	maxPerPR     = 500
	maxBody      = 65536
	maxPathBytes = 1024
)

var (
	keyRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*#[0-9]{1,9}$`)
	headRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

// Draft is one unsent comment: a line, or a run of lines when StartLine is set, on one side of one
// file's diff. Side is "LEFT" (the old file) or "RIGHT" (the new one).
type Draft struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	StartLine int    `json:"start_line,omitempty"`
	Side      string `json:"side"`
	Body      string `json:"body"`
}

type entry struct {
	Head   string  `json:"head"`
	Drafts []Draft `json:"drafts"`
}

// Store is the drafts of every PR. A nil *Store works and remembers nothing.
type Store struct {
	mu    sync.Mutex
	path  string
	items map[string]entry
}

// DefaultPath is the per-user file next to favorites.json.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "drafts.json")
}

// Key names a PR: host/owner/name#number. An empty host means github.com.
func Key(host, owner, name, number string) string {
	if host == "" {
		host = "github.com"
	}
	return host + "/" + owner + "/" + name + "#" + number
}

func validDraft(d Draft) error {
	switch {
	case d.Path == "" || len(d.Path) > maxPathBytes || safe.Text(d.Path) != d.Path || strings.ContainsAny(d.Path, "\n\t"):
		return errors.New("bad file name")
	case d.Line < 1:
		return errors.New("bad line")
	case d.Side != "LEFT" && d.Side != "RIGHT":
		return errors.New("bad side")
	case d.StartLine != 0 && (d.StartLine < 1 || d.StartLine >= d.Line):
		return errors.New("a range has to run forward")
	case strings.TrimSpace(d.Body) == "" || len(d.Body) > maxBody:
		return errors.New("bad comment text")
	case safe.Text(d.Body) != d.Body:
		return errors.New("the comment holds control characters")
	}
	return nil
}

func validEntry(key string, e entry) error {
	switch {
	case !keyRe.MatchString(key):
		return fmt.Errorf("bad pull request name %q", key)
	case e.Head != "" && !headRe.MatchString(e.Head):
		return errors.New("bad commit")
	case len(e.Drafts) > maxPerPR:
		return errors.New("too many drafts")
	}
	for i, d := range e.Drafts {
		if err := validDraft(d); err != nil {
			return fmt.Errorf("draft %d: %w", i+1, err)
		}
	}
	return nil
}

// Load reads the store. A missing or empty file is an empty store; an unparseable or invalid file
// is an error and the file is left untouched.
func Load(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]entry{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && strings.TrimSpace(string(b)) == "") {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.items); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(s.items) > maxPRs {
		return nil, fmt.Errorf("%s: too many pull requests", path)
	}
	for k, e := range s.items {
		if err := validEntry(k, e); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return s, nil
}

// Get returns the commit the drafts were written against and the drafts of a PR.
func (s *Store) Get(key string) (head string, ds []Draft) {
	if s == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.items[key]
	return e.Head, append([]Draft(nil), e.Drafts...)
}

// Put replaces the drafts of a PR and saves the file; an empty list removes the PR.
func (s *Store) Put(key, head string, ds []Draft) error {
	if s == nil {
		return nil
	}
	e := entry{Head: head, Drafts: append([]Draft(nil), ds...)}
	if err := validEntry(key, e); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]entry, len(s.items)+1)
	for k, v := range s.items {
		next[k] = v
	}
	if len(ds) == 0 {
		delete(next, key)
	} else {
		if _, known := next[key]; !known && len(next) >= maxPRs {
			return errors.New("too many pull requests with drafts")
		}
		next[key] = e
	}
	if err := save(s.path, next); err != nil {
		return err
	}
	s.items = next
	return nil
}

// Delete forgets a PR's drafts.
func (s *Store) Delete(key string) error { return s.Put(key, "", nil) }

func save(path string, items map[string]entry) error {
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".drafts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
