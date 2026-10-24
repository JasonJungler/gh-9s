package ghcli

import (
	"encoding/json"
	"sort"
)

// ThreadComment is one comment of a review thread.
type ThreadComment struct{ Author, Body, CreatedAt string }

// Thread is a conversation other people started on a line of the diff.
type Thread struct {
	Path     string
	Line     int
	Side     string // "LEFT" (the old file) or "RIGHT" (the new one)
	Outdated bool   // the line is no longer in the diff; Line is where it was
	Comments []ThreadComment
}

// ParseThreads reads the REST list of a PR's review comments (one array, or the array of pages
// that `gh api --slurp` gives) and groups replies under the comment they answer.
func ParseThreads(b []byte) ([]Thread, error) {
	type raw struct {
		ID           int64  `json:"id"`
		InReplyTo    int64  `json:"in_reply_to_id"`
		Path         string `json:"path"`
		Line         *int   `json:"line"`
		OriginalLine int    `json:"original_line"`
		Side         string `json:"side"`
		User         struct {
			Login string `json:"login"`
		} `json:"user"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	var flat []raw
	if err := json.Unmarshal(b, &flat); err != nil {
		flat = nil // a failed decode can leave empty entries behind
		var pages [][]raw
		if err2 := json.Unmarshal(b, &pages); err2 != nil {
			return nil, err
		}
		for _, p := range pages {
			flat = append(flat, p...)
		}
	}
	var threads []Thread
	byID := map[int64]int{} // comment id -> index in threads
	for _, r := range flat {
		c := ThreadComment{Author: CleanText(r.User.Login), Body: CleanText(r.Body), CreatedAt: r.CreatedAt}
		if at, ok := byID[r.InReplyTo]; ok && r.InReplyTo != 0 {
			threads[at].Comments = append(threads[at].Comments, c)
			byID[r.ID] = at
			continue
		}
		t := Thread{Path: CleanText(r.Path), Side: r.Side, Comments: []ThreadComment{c}}
		if t.Side == "" {
			t.Side = "RIGHT"
		}
		if r.Line != nil {
			t.Line = *r.Line
		} else {
			t.Line, t.Outdated = r.OriginalLine, true
		}
		threads = append(threads, t)
		byID[r.ID] = len(threads) - 1
	}
	for i := range threads {
		cs := threads[i].Comments
		sort.SliceStable(cs, func(a, b int) bool { return cs[a].CreatedAt < cs[b].CreatedAt })
	}
	return threads, nil
}

// ThreadsArgs is the `gh api` call that lists a PR's review comments, every page in one array.
func ThreadsArgs(r RepoRef, number string) []string {
	args := []string{"api"}
	if r.Host != "" && r.Host != "github.com" {
		args = append(args, "--hostname", r.Host)
	}
	repo := "{owner}/{repo}" // gh fills this in from the current directory
	if r.Owner != "" {
		repo = r.Owner + "/" + r.Name
	}
	return append(args, "--paginate", "--slurp", "repos/"+repo+"/pulls/"+number+"/comments?per_page=100")
}

// FetchReviewThreads lists the inline review comments other people left on the PR.
func FetchReviewThreads(r RepoRef, number string) ([]Thread, error) {
	out, err := Run(ThreadsArgs(r, number)...)
	if err != nil {
		return nil, err
	}
	return ParseThreads(out)
}
