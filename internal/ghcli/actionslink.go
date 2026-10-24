package ghcli

import (
	"net/url"
	"regexp"
	"strings"
)

// ActionsLink points at a GitHub Actions run, optionally at one of its jobs.
type ActionsLink struct {
	Repo         RepoRef
	RunID, JobID string
}

var (
	digits    = regexp.MustCompile(`^[0-9]+$`)
	linkToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ParseActionsURL recognises https://HOST/OWNER/REPO/actions/runs/<run>[/job/<job>] (a trailing
// slash, attempts, a query or a fragment are fine). Any other URL, such as an external CI link,
// an old-style check-run link or a workflow page, is not an Actions run and returns false.
func ParseActionsURL(raw string) (ActionsLink, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ActionsLink{}, false
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	// owner repo actions runs <run> [job <job> | attempts <n>]
	if len(p) < 5 || p[2] != "actions" || p[3] != "runs" {
		return ActionsLink{}, false
	}
	if !linkToken.MatchString(p[0]) || !linkToken.MatchString(p[1]) || !digits.MatchString(p[4]) {
		return ActionsLink{}, false
	}
	l := ActionsLink{Repo: RepoRef{Host: u.Host, Owner: p[0], Name: p[1]}, RunID: p[4]}
	if len(p) >= 7 && p[5] == "job" && digits.MatchString(p[6]) {
		l.JobID = p[6]
	}
	return l, true
}
