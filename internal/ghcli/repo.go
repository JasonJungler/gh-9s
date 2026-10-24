package ghcli

import (
	"fmt"
	"net/url"
	"strings"
)

// RepoRef names a repository on a host. The zero value means "the repository
// of the current directory", which is what gh resolves when given no -R.
type RepoRef struct{ Host, Owner, Name string }

// ParseRepoRef parses OWNER/REPO or HOST/OWNER/REPO.
func ParseRepoRef(s string) (RepoRef, error) {
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" {
			return RepoRef{}, fmt.Errorf("invalid repo %q: want [HOST/]OWNER/REPO", s)
		}
	}
	switch len(parts) {
	case 2:
		return RepoRef{Owner: parts[0], Name: parts[1]}, nil
	case 3:
		return RepoRef{Host: parts[0], Owner: parts[1], Name: parts[2]}, nil
	}
	return RepoRef{}, fmt.Errorf("invalid repo %q: want [HOST/]OWNER/REPO", s)
}

// String renders OWNER/REPO, HOST/OWNER/REPO, or "" for the zero value.
func (r RepoRef) String() string {
	switch {
	case r == RepoRef{}:
		return ""
	case r.Host == "":
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

// Args returns the -R flag for r, or nil for the zero value.
func (r RepoRef) Args() []string {
	if r == (RepoRef{}) {
		return nil
	}
	return []string{"-R", r.String()}
}

// Scoped appends the -R flag for r (if any) to a gh argument list.
func Scoped(r RepoRef, args ...string) []string {
	return append(args, r.Args()...)
}

// CurrentRepo resolves the repository of the current directory, including its host.
func CurrentRepo() (RepoRef, error) {
	var v struct {
		URL string `json:"url"`
	}
	if err := JSON(&v, "repo", "view", "--json", "url"); err != nil {
		return RepoRef{}, err
	}
	u, err := url.Parse(v.URL)
	if err != nil {
		return RepoRef{}, err
	}
	r, err := ParseRepoRef(u.Host + u.Path)
	if err != nil {
		return RepoRef{}, err
	}
	return r, nil
}

// APIEndpoint builds a `gh api` endpoint under this repo plus any flags it needs
// (--hostname for non-default hosts). The zero repoRef uses gh's {owner}/{repo}
// placeholders, which resolve to the current directory's repo.
func (r RepoRef) APIEndpoint(rest string) (endpoint string, flags []string) {
	repo := "{owner}/{repo}"
	if r != (RepoRef{}) {
		repo = r.Owner + "/" + r.Name
		if r.Host != "" {
			flags = []string{"--hostname", r.Host}
		}
	}
	return "repos/" + repo + rest, flags
}
