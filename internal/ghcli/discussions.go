package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Discussion limits: what one read takes. More is noted on screen instead of paged.
const (
	DiscussionListLimit = 50  // discussions in the list
	DiscussionComments  = 100 // top-level comments of one discussion
	DiscussionReplies   = 50  // replies read under each comment
	discBodyCut         = 4000
)

// discRun is the seam for tests.
var discRun = RunWithTimeout

const discListQuery = `query($owner:String!,$name:String!,$cat:ID){repository(owner:$owner,name:$name){hasDiscussionsEnabled discussionCategories(first:25){nodes{id name isAnswerable}} discussions(first:50,categoryId:$cat,orderBy:{field:UPDATED_AT,direction:DESC}){totalCount nodes{id number title body url updatedAt closed locked isAnswered author{login} category{name} comments{totalCount}}}}}`

const discQuery = `query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){hasDiscussionsEnabled discussion(number:$n){id number title body url closed locked isAnswered createdAt author{login} category{name} comments(first:100){totalCount nodes{id body createdAt isAnswer author{login} replies(first:50){totalCount nodes{id body createdAt author{login}}}}}}}}`

// DiscCategory is one category of a repo's Discussions.
type DiscCategory struct {
	ID, Name   string
	Answerable bool
}

// DiscussionRow is one line of the list.
type DiscussionRow struct {
	ID                       string
	Number                   int
	Title, Body, Author      string
	Category, URL, UpdatedAt string
	Closed, Locked, Answered bool
	Comments                 int
}

// DiscussionList is the list screen's data.
type DiscussionList struct {
	Enabled    bool
	Categories []DiscCategory
	Total      int
	Rows       []DiscussionRow
}

// DiscReply is a reply under a comment.
type DiscReply struct{ ID, Author, Body, CreatedAt string }

// DiscComment is a top-level comment with the replies that were read.
type DiscComment struct {
	ID, Author, Body, CreatedAt string
	IsAnswer                    bool
	Replies                     []DiscReply
	ReplyTotal                  int
}

// DiscussionDetail is what the discussion screen shows.
type DiscussionDetail struct {
	ID                                 string
	Number                             int
	Title, Body, Author, Category, URL string
	CreatedAt                          string
	Closed, Locked, Answered, Enabled  bool
	Comments                           []DiscComment
	CommentTotal                       int
}

type discLogin struct {
	Login string `json:"login"`
}

func discAuthor(a *discLogin) string {
	if a == nil || a.Login == "" {
		return "ghost"
	}
	return CleanText(a.Login)
}

func cutBody(s string) string {
	s = CleanText(s)
	if r := []rune(s); len(r) > discBodyCut {
		return string(r[:discBodyCut]) + "…"
	}
	return s
}

// discGraphQLErr turns the errors of an answer without data into an error.
func discGraphQLErr(errs gqlErrors) error {
	if len(errs) == 0 {
		return errors.New("no data in the response")
	}
	return errors.New(oneLine(CleanText(errs[0].Message)))
}

// ParseDiscussionList reads the answer to discListQuery.
func ParseDiscussionList(b []byte) (DiscussionList, error) {
	var resp struct {
		Data *struct {
			Repo *struct {
				Enabled bool `json:"hasDiscussionsEnabled"`
				Cats    struct {
					Nodes []struct {
						ID         string `json:"id"`
						Name       string `json:"name"`
						Answerable bool   `json:"isAnswerable"`
					} `json:"nodes"`
				} `json:"discussionCategories"`
				Discussions struct {
					Total int `json:"totalCount"`
					Nodes []*struct {
						ID        string     `json:"id"`
						Number    int        `json:"number"`
						Title     string     `json:"title"`
						Body      string     `json:"body"`
						URL       string     `json:"url"`
						UpdatedAt string     `json:"updatedAt"`
						Closed    bool       `json:"closed"`
						Locked    bool       `json:"locked"`
						Answered  bool       `json:"isAnswered"`
						Author    *discLogin `json:"author"`
						Category  *struct {
							Name string `json:"name"`
						} `json:"category"`
						Comments struct {
							Total int `json:"totalCount"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"discussions"`
			} `json:"repository"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return DiscussionList{}, err
	}
	if resp.Data == nil || resp.Data.Repo == nil {
		return DiscussionList{}, discGraphQLErr(resp.Errors)
	}
	r := resp.Data.Repo
	out := DiscussionList{Enabled: r.Enabled, Total: r.Discussions.Total}
	for _, c := range r.Cats.Nodes {
		if c.ID != "" && c.Name != "" {
			out.Categories = append(out.Categories, DiscCategory{c.ID, oneLine(CleanText(c.Name)), c.Answerable})
		}
	}
	for _, n := range r.Discussions.Nodes {
		if n == nil {
			continue
		}
		row := DiscussionRow{ID: n.ID, Number: n.Number, Title: oneLine(CleanText(n.Title)), Body: cutBody(n.Body), Author: discAuthor(n.Author),
			URL: CleanText(n.URL), UpdatedAt: n.UpdatedAt, Closed: n.Closed, Locked: n.Locked, Answered: n.Answered, Comments: n.Comments.Total}
		if n.Category != nil {
			row.Category = oneLine(CleanText(n.Category.Name))
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// DiscListArgs is the gh call that reads the list; the query and every value are separate arguments.
func DiscListArgs(host string, r RepoRef, categoryID string) []string {
	args := append(graphqlArgs(host), "-f", "query="+discListQuery, "-f", "owner="+r.Owner, "-f", "name="+r.Name)
	if categoryID != "" {
		args = append(args, "-f", "cat="+categoryID)
	}
	return args
}

// FetchDiscussions reads the repo's discussions, newest activity first, of one category when categoryID is set.
func FetchDiscussions(host string, r RepoRef, categoryID string) (DiscussionList, error) {
	var out []byte
	err := retryOnce(func() error {
		var e error
		out, e = discRun(inboxTimeout, DiscListArgs(host, r, categoryID)...)
		if e != nil && len(out) == 0 {
			return e
		}
		return nil
	})
	if err != nil {
		return DiscussionList{}, err
	}
	return ParseDiscussionList(out)
}

// ParseDiscussion reads the answer to discQuery.
func ParseDiscussion(b []byte) (DiscussionDetail, error) {
	var resp struct {
		Data *struct {
			Repo *struct {
				Enabled    bool `json:"hasDiscussionsEnabled"`
				Discussion *struct {
					ID        string     `json:"id"`
					Number    int        `json:"number"`
					Title     string     `json:"title"`
					Body      string     `json:"body"`
					URL       string     `json:"url"`
					Closed    bool       `json:"closed"`
					Locked    bool       `json:"locked"`
					Answered  bool       `json:"isAnswered"`
					CreatedAt string     `json:"createdAt"`
					Author    *discLogin `json:"author"`
					Category  *struct {
						Name string `json:"name"`
					} `json:"category"`
					Comments struct {
						Total int `json:"totalCount"`
						Nodes []*struct {
							ID        string     `json:"id"`
							Body      string     `json:"body"`
							CreatedAt string     `json:"createdAt"`
							IsAnswer  bool       `json:"isAnswer"`
							Author    *discLogin `json:"author"`
							Replies   struct {
								Total int `json:"totalCount"`
								Nodes []*struct {
									ID        string     `json:"id"`
									Body      string     `json:"body"`
									CreatedAt string     `json:"createdAt"`
									Author    *discLogin `json:"author"`
								} `json:"nodes"`
							} `json:"replies"`
						} `json:"nodes"`
					} `json:"comments"`
				} `json:"discussion"`
			} `json:"repository"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return DiscussionDetail{}, err
	}
	if resp.Data == nil || resp.Data.Repo == nil || resp.Data.Repo.Discussion == nil {
		if resp.Data != nil && resp.Data.Repo != nil && !resp.Data.Repo.Enabled {
			return DiscussionDetail{}, errors.New("Discussions are not enabled in this repo")
		}
		return DiscussionDetail{}, discGraphQLErr(resp.Errors)
	}
	n := resp.Data.Repo.Discussion
	d := DiscussionDetail{ID: n.ID, Number: n.Number, Title: oneLine(CleanText(n.Title)), Body: CleanText(n.Body), Author: discAuthor(n.Author),
		URL: CleanText(n.URL), CreatedAt: n.CreatedAt, Closed: n.Closed, Locked: n.Locked, Answered: n.Answered, Enabled: true, CommentTotal: n.Comments.Total}
	if n.Category != nil {
		d.Category = oneLine(CleanText(n.Category.Name))
	}
	for _, c := range n.Comments.Nodes {
		if c == nil {
			continue
		}
		dc := DiscComment{ID: c.ID, Author: discAuthor(c.Author), Body: CleanText(c.Body), CreatedAt: c.CreatedAt, IsAnswer: c.IsAnswer, ReplyTotal: c.Replies.Total}
		for _, r := range c.Replies.Nodes {
			if r != nil {
				dc.Replies = append(dc.Replies, DiscReply{r.ID, discAuthor(r.Author), CleanText(r.Body), r.CreatedAt})
			}
		}
		d.Comments = append(d.Comments, dc)
	}
	return d, nil
}

// DiscArgs is the gh call that reads one discussion.
func DiscArgs(host string, r RepoRef, number int) []string {
	return append(graphqlArgs(host), "-f", "query="+discQuery, "-f", "owner="+r.Owner, "-f", "name="+r.Name, "-F", fmt.Sprintf("n=%d", number))
}

// FetchDiscussion reads one discussion with its comments and their replies.
func FetchDiscussion(host string, r RepoRef, number string) (DiscussionDetail, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(number), "%d", &n); err != nil || n <= 0 {
		return DiscussionDetail{}, fmt.Errorf("%q is not a discussion number", number)
	}
	var out []byte
	err := retryOnce(func() error {
		var e error
		out, e = discRun(inboxTimeout, DiscArgs(host, r, n)...)
		if e != nil && len(out) == 0 {
			return e
		}
		return nil
	})
	if err != nil {
		return DiscussionDetail{}, err
	}
	return ParseDiscussion(out)
}
