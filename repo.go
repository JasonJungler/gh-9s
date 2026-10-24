package main

import (
	"fmt"
	"net/url"
	"strings"
)

// repoRef names a repository on a host. The zero value means "the repository
// of the current directory", which is what gh resolves when given no -R.
type repoRef struct{ Host, Owner, Name string }

func parseRepoRef(s string) (repoRef, error) {
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" {
			return repoRef{}, fmt.Errorf("invalid repo %q: want [HOST/]OWNER/REPO", s)
		}
	}
	switch len(parts) {
	case 2:
		return repoRef{Owner: parts[0], Name: parts[1]}, nil
	case 3:
		return repoRef{Host: parts[0], Owner: parts[1], Name: parts[2]}, nil
	}
	return repoRef{}, fmt.Errorf("invalid repo %q: want [HOST/]OWNER/REPO", s)
}

func (r repoRef) String() string {
	switch {
	case r == repoRef{}:
		return ""
	case r.Host == "":
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

func (r repoRef) args() []string {
	if r == (repoRef{}) {
		return nil
	}
	return []string{"-R", r.String()}
}

// scoped appends the -R flag for r (if any) to a gh argument list.
func scoped(r repoRef, args ...string) []string {
	return append(args, r.args()...)
}

// currentRepo resolves the repository of the current directory, including its host.
func currentRepo() (repoRef, error) {
	var v struct {
		URL string `json:"url"`
	}
	if err := ghJSON(&v, "repo", "view", "--json", "url"); err != nil {
		return repoRef{}, err
	}
	u, err := url.Parse(v.URL)
	if err != nil {
		return repoRef{}, err
	}
	r, err := parseRepoRef(u.Host + u.Path)
	if err != nil {
		return repoRef{}, err
	}
	return r, nil
}

// apiEndpoint builds a `gh api` endpoint under this repo plus any flags it needs
// (--hostname for non-default hosts). The zero repoRef uses gh's {owner}/{repo}
// placeholders, which resolve to the current directory's repo.
func (r repoRef) apiEndpoint(rest string) (endpoint string, flags []string) {
	repo := "{owner}/{repo}"
	if r != (repoRef{}) {
		repo = r.Owner + "/" + r.Name
		if r.Host != "" {
			flags = []string{"--hostname", r.Host}
		}
	}
	return "repos/" + repo + rest, flags
}
