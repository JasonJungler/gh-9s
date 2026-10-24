package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

// InboxTab is one of the lists of the inbox.
type InboxTab int

const (
	InboxAssigned InboxTab = iota
	InboxReviewRequested
	InboxMine
	InboxMentions
)

// InboxItem is an issue or pull request found by an inbox search.
type InboxItem struct {
	Host, Repo     string // Repo is OWNER/NAME
	Number         int
	Kind           string // "issue" or "pr"
	Title, Author  string
	URL, UpdatedAt string
	Draft          bool
	ReviewDecision string // PRs: APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or ""
	Checks         string // PRs: SUCCESS, FAILURE, PENDING, ERROR, EXPECTED, or ""
}

var (
	hostRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]{1,5})?$`)
	loginRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(\[bot\])?$`)
)

// ParseAuthHosts reads `gh auth status --json hosts`: the hosts gh is logged in to, github.com first.
func ParseAuthHosts(b []byte) []string {
	var v struct {
		Hosts map[string]json.RawMessage `json:"hosts"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	var rest []string
	first := false
	for h := range v.Hosts {
		switch {
		case !hostRe.MatchString(h):
		case h == "github.com":
			first = true
		default:
			rest = append(rest, h)
		}
	}
	sort.Strings(rest)
	if first {
		return append([]string{"github.com"}, rest...)
	}
	return rest
}

// InboxHosts lists the hosts to search. If gh cannot say (an older gh, nothing logged in) it is
// just github.com and the error says why.
func InboxHosts() ([]string, error) {
	out, err := RunWithTimeout(inboxTimeout, "auth", "status", "--json", "hosts")
	if hosts := ParseAuthHosts(out); len(hosts) > 0 {
		return hosts, nil
	}
	if err == nil {
		err = errors.New("gh lists no logged-in hosts")
	}
	return []string{"github.com"}, err
}

// InboxSearch is GitHub's search string for a tab; "" for a login that does not look like one, so
// nothing but a plain login is ever put into a query.
func InboxSearch(tab InboxTab, login string) string {
	if !loginRe.MatchString(login) {
		return ""
	}
	const tail = " archived:false sort:updated-desc"
	switch tab {
	case InboxAssigned:
		return "is:issue is:open assignee:" + login + tail
	case InboxReviewRequested:
		return "is:pr is:open review-requested:" + login + tail
	case InboxMine:
		return "is:pr is:open author:" + login + tail
	case InboxMentions:
		return "is:open mentions:" + login + " -author:" + login + tail
	}
	return ""
}

const inboxQuery = `query($q:String!,$n:Int!){search(query:$q,type:ISSUE,first:$n){issueCount nodes{__typename ` +
	`... on Issue{number title url updatedAt author{login} repository{nameWithOwner}} ` +
	`... on PullRequest{number title url updatedAt isDraft reviewDecision author{login} repository{nameWithOwner} ` +
	`commits(last:1){nodes{commit{statusCheckRollup{state}}}}}}}}`

// inboxTimeout is how long one call to a host may take.
const inboxTimeout = 30 * time.Second

// InboxLimit is how many rows one search asks for.
const InboxLimit = 100

func graphqlArgs(host string) []string {
	args := []string{"api", "graphql"}
	if host != "" { // always named, github.com too: a GH_HOST in the environment must not redirect the search
		args = append(args, "--hostname", host)
	}
	return args
}

// InboxArgs is the gh call that runs one search; the query and the search are separate arguments.
func InboxArgs(host, search string) []string {
	return append(graphqlArgs(host), "-f", "query="+inboxQuery, "-f", "q="+search, "-F", fmt.Sprintf("n=%d", InboxLimit))
}

type gqlErrors []struct {
	Message string `json:"message"`
}

// ParseInbox reads a search response. Errors with no data are an error; errors next to data (a
// repository the user cannot see any more) still give what was found.
func ParseInbox(host string, b []byte) (items []InboxItem, total int, err error) {
	var resp struct {
		Data *struct {
			Search struct {
				IssueCount int `json:"issueCount"`
				Nodes      []*struct {
					Type      string `json:"__typename"`
					Number    int    `json:"number"`
					Title     string `json:"title"`
					URL       string `json:"url"`
					UpdatedAt string `json:"updatedAt"`
					IsDraft   bool   `json:"isDraft"`
					Decision  string `json:"reviewDecision"`
					Author    *struct {
						Login string `json:"login"`
					} `json:"author"`
					Repo *struct {
						Name string `json:"nameWithOwner"`
					} `json:"repository"`
					Commits *struct {
						Nodes []struct {
							Commit struct {
								Rollup *struct {
									State string `json:"state"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, 0, err
	}
	if resp.Data == nil {
		if len(resp.Errors) > 0 {
			return nil, 0, errors.New(CleanText(resp.Errors[0].Message))
		}
		return nil, 0, errors.New("no data in the response")
	}
	if host == "" {
		host = "github.com"
	}
	for _, n := range resp.Data.Search.Nodes {
		if n == nil || n.Repo == nil || n.Number == 0 || (n.Type != "Issue" && n.Type != "PullRequest") {
			continue
		}
		it := InboxItem{Host: host, Repo: CleanText(n.Repo.Name), Number: n.Number, Title: oneLine(CleanText(n.Title)),
			URL: CleanText(n.URL), UpdatedAt: n.UpdatedAt, Author: "ghost", Kind: "issue"}
		if n.Author != nil && n.Author.Login != "" {
			it.Author = CleanText(n.Author.Login)
		}
		if n.Type == "PullRequest" {
			it.Kind, it.Draft, it.ReviewDecision = "pr", n.IsDraft, n.Decision
			if n.Commits != nil && len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.Rollup != nil {
				it.Checks = n.Commits.Nodes[0].Commit.Rollup.State
			}
		}
		items = append(items, it)
	}
	return items, resp.Data.Search.IssueCount, nil
}

// oneLine keeps a title on one line.
func oneLine(s string) string { return newlines.ReplaceAllString(s, " ") }

var newlines = regexp.MustCompile(`\s*\n\s*`)

// FetchInbox runs one tab's search on one host.
func FetchInbox(host string, tab InboxTab, login string) ([]InboxItem, int, error) {
	search := InboxSearch(tab, login)
	if search == "" {
		return nil, 0, errors.New("no usable login for this host")
	}
	out, err := RunWithTimeout(inboxTimeout, InboxArgs(host, search)...)
	if err != nil && len(out) == 0 {
		return nil, 0, err
	}
	return ParseInbox(host, out)
}

// LoginArgs asks a host who the logged-in user is.
func LoginArgs(host string) []string {
	return append(graphqlArgs(host), "-f", "query={viewer{login}}")
}

// ParseLogin reads the answer to LoginArgs.
func ParseLogin(b []byte) (string, error) {
	var resp struct {
		Data *struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return "", err
	}
	if resp.Data == nil {
		if len(resp.Errors) > 0 {
			return "", errors.New(CleanText(resp.Errors[0].Message))
		}
		return "", errors.New("no data in the response")
	}
	if !loginRe.MatchString(resp.Data.Viewer.Login) {
		return "", errors.New("the host returned a login that is not a login")
	}
	return resp.Data.Viewer.Login, nil
}

// FetchLogin is the logged-in user's login on a host.
func FetchLogin(host string) (string, error) {
	out, err := RunWithTimeout(inboxTimeout, LoginArgs(host)...)
	if err != nil && len(out) == 0 {
		return "", err
	}
	return ParseLogin(out)
}
