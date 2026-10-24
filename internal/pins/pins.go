// Package pins stores the repos and GitHub Projects the user pinned in the favorites screen, in a
// personal config file next to favorites.json. Everything read back is validated, because the file
// may be hand-edited or synced and its values end up in command lines.
package pins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// Repo is a pinned repository; Host is "" for github.com.
type Repo struct {
	Host string `json:"host"`
	Repo string `json:"repo"` // owner/name
}

// Project is a pinned GitHub Project (v2); Title is what it was called when it was pinned.
type Project struct {
	Host   string `json:"host"`
	Owner  string `json:"owner"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type file struct {
	Repos    []Repo    `json:"repos"`
	Projects []Project `json:"projects"`
}

// Store is the pinned repos and projects, persisted as JSON outside any repo.
type Store struct {
	path     string
	Repos    []Repo
	Projects []Project

	// removed since the last Save, so that re-reading the file does not bring them back
	removedRepo    []Repo
	removedProject []Project
}

var token = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func okToken(s string) bool { return token.MatchString(s) && s != "." && s != ".." }

func normHost(h string) string {
	if strings.EqualFold(h, "github.com") {
		return ""
	}
	return strings.ToLower(h)
}

func (r Repo) validate() error {
	owner, name, ok := strings.Cut(r.Repo, "/")
	switch {
	case !ok || !okToken(owner) || !okToken(name):
		return fmt.Errorf("invalid repo %q (want owner/name)", r.Repo)
	case r.Host != "" && !okToken(r.Host):
		return fmt.Errorf("invalid host %q", r.Host)
	}
	return nil
}

func (p Project) validate() error {
	switch {
	case !okToken(p.Owner):
		return fmt.Errorf("invalid project owner %q", p.Owner)
	case p.Number < 1:
		return fmt.Errorf("invalid project number %d", p.Number)
	case p.Host != "" && !okToken(p.Host):
		return fmt.Errorf("invalid host %q", p.Host)
	}
	return nil
}

func (r Repo) same(o Repo) bool {
	return normHost(r.Host) == normHost(o.Host) && strings.EqualFold(r.Repo, o.Repo)
}

func (p Project) same(host, owner string, n int) bool {
	return normHost(p.Host) == normHost(host) && strings.EqualFold(p.Owner, owner) && p.Number == n
}

// Target is the repo as a RepoRef.
func (r Repo) Target() ghcli.RepoRef {
	owner, name, _ := strings.Cut(r.Repo, "/")
	return ghcli.RepoRef{Host: r.Host, Owner: owner, Name: name}
}

// ParseRepo reads [HOST/]OWNER/NAME.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	var host string
	switch len(parts) {
	case 2:
	case 3:
		host, parts = parts[0], parts[1:]
	default:
		return Repo{}, fmt.Errorf("invalid repo %q (want [HOST/]OWNER/NAME)", s)
	}
	r := Repo{Host: normHost(host), Repo: parts[0] + "/" + parts[1]}
	return r, r.validate()
}

// ParseProject reads [HOST/]OWNER/NUMBER.
func ParseProject(s string) (host, owner string, n int, err error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	switch len(parts) {
	case 2:
	case 3:
		host, parts = parts[0], parts[1:]
	default:
		return "", "", 0, fmt.Errorf("invalid project %q (want [HOST/]OWNER/NUMBER)", s)
	}
	n, convErr := strconv.Atoi(parts[1])
	if convErr != nil || strconv.Itoa(n) != parts[1] {
		return "", "", 0, fmt.Errorf("invalid project number %q", parts[1])
	}
	p := Project{Host: normHost(host), Owner: parts[0], Number: n}
	if err := p.validate(); err != nil {
		return "", "", 0, err
	}
	return p.Host, p.Owner, n, nil
}

// DefaultPath is the per-user file; GH9S_PINS overrides it.
func DefaultPath() string {
	if p := os.Getenv("GH9S_PINS"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "pins.json")
}

func read(path string) (file, error) {
	var f file
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(b))) == 0) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	for i, r := range f.Repos {
		if err := r.validate(); err != nil {
			return file{}, fmt.Errorf("%s: repo %d: %w", path, i+1, err)
		}
	}
	for i, p := range f.Projects {
		if err := p.validate(); err != nil {
			return file{}, fmt.Errorf("%s: project %d: %w", path, i+1, err)
		}
	}
	return f, nil
}

// Load reads the store. A missing or empty file is an empty store; an unreadable or invalid file is an
// error and is never rewritten.
func Load(path string) (*Store, error) {
	f, err := read(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, Repos: f.Repos, Projects: f.Projects}, nil
}

// Memory is a store that is not backed by a file (for when the file is broken): Save fails.
func Memory() *Store { return &Store{} }

// Path is the file the store reads and writes.
func (s *Store) Path() string { return s.path }

// Save writes the store atomically. The file is read again first, so a pin another gh-9s added since
// this store was loaded is kept; if that file is invalid nothing is written.
func (s *Store) Save() error {
	if s.path == "" {
		return errors.New("pins are not saved: no usable file")
	}
	disk, err := read(s.path)
	if err != nil {
		return err
	}
	// keep what is on disk that this store never had and did not remove: only additions of the
	// other instance survive, which is the case that matters (two terminals pinning)
	for _, r := range disk.Repos {
		if !s.HasRepo(r.Host, r.Repo) && !containsRepo(s.removedRepo, r) {
			s.Repos = append(s.Repos, r)
		}
	}
	for _, p := range disk.Projects {
		if !s.HasProject(p.Host, p.Owner, p.Number) && !containsProject(s.removedProject, p) {
			s.Projects = append(s.Projects, p)
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(file{Repos: nonNilRepos(s.Repos), Projects: nonNilProjects(s.Projects)}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.removedRepo, s.removedProject = nil, nil
	return nil
}

func containsRepo(l []Repo, r Repo) bool {
	for _, x := range l {
		if x.same(r) {
			return true
		}
	}
	return false
}

func containsProject(l []Project, p Project) bool {
	for _, x := range l {
		if x.same(p.Host, p.Owner, p.Number) {
			return true
		}
	}
	return false
}

func nonNilRepos(l []Repo) []Repo {
	if l == nil {
		return []Repo{}
	}
	return l
}

func nonNilProjects(l []Project) []Project {
	if l == nil {
		return []Project{}
	}
	return l
}

// HasRepo reports whether the repo is pinned.
func (s *Store) HasRepo(host, repo string) bool {
	return containsRepo(s.Repos, Repo{Host: host, Repo: repo})
}

// HasProject reports whether the project is pinned.
func (s *Store) HasProject(host, owner string, n int) bool {
	return containsProject(s.Projects, Project{Host: host, Owner: owner, Number: n})
}

// ToggleRepo pins the repo, or unpins it when it already is; it reports whether it is pinned now.
func (s *Store) ToggleRepo(r Repo) (bool, error) {
	if err := r.validate(); err != nil {
		return false, err
	}
	r.Host = normHost(r.Host)
	for i, x := range s.Repos {
		if x.same(r) {
			s.Repos = append(s.Repos[:i:i], s.Repos[i+1:]...)
			s.removedRepo = append(s.removedRepo, x)
			return false, nil
		}
	}
	s.Repos = append(s.Repos, r)
	return true, nil
}

// ToggleProject pins the project, or unpins it when it already is; it reports whether it is pinned now.
func (s *Store) ToggleProject(p Project) (bool, error) {
	if err := p.validate(); err != nil {
		return false, err
	}
	p.Host = normHost(p.Host)
	for i, x := range s.Projects {
		if x.same(p.Host, p.Owner, p.Number) {
			s.Projects = append(s.Projects[:i:i], s.Projects[i+1:]...)
			s.removedProject = append(s.removedProject, x)
			return false, nil
		}
	}
	s.Projects = append(s.Projects, p)
	return true, nil
}
