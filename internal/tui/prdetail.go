package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/icons"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls the PR screens make; tests replace them.
var (
	fetchPR = ghcli.FetchPR
	openURL = ghcli.OpenURL
)

// open sends url to the browser; when that fails the reason is shown instead of lost.
func (m model) open(url string) model {
	if err := openURL(url); err != nil {
		m.status = "can't open the browser: " + err.Error()
	}
	return m
}

// prDetailState is what the PR detail screen keeps in the model.
type prDetailState struct {
	d      ghcli.PRDetail
	scroll int
	img    int // selected image marker, 1+index; 0 none
}

func init() {
	register(viewPRDetail, screen{
		name:     "pr",
		identity: func(m model) string { return m.itemID(m.pr) },
		fetch: func(m model) tea.Cmd {
			tg, number, id := m.target, m.pr, m.itemID(m.pr)
			return func() tea.Msg {
				d, err := fetchPR(tg, number)
				return screenMsg{view: viewPRDetail, id: id, data: d, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			old, _ := m.screenData(viewPRDetail).(prDetailState)
			return m.withScreenData(viewPRDetail, prDetailState{d: msg.data.(ghcli.PRDetail), scroll: old.scroll, img: old.img})
		},
		key:    prDetailKey,
		body:   prDetailBody,
		footer: "c checks  d diff  C comment  r review  m merge  M projects  o open  j/k scroll",
	})
}

func prDetailKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewPRDetail).(prDetailState)
	h := m.logHeight()
	doc := prDetailLines(st.d, m.contentWidth())
	maxScroll := max(0, len(doc.lines)-h)
	var act imageAction
	if st.img, st.scroll, act = imageKeys(doc, st.img, st.scroll, h, k.String()); act != imgNone {
		m = m.withScreenData(viewPRDetail, st)
		switch act {
		case imgShow:
			next, cmd := showImage(m, doc.images[st.img-1].URL)
			return next, cmd, true
		case imgOpen:
			return m.openImageLink(doc.images[st.img-1].URL), nil, true
		}
		return m, nil, true
	}
	switch k.String() {
	case "j", "down":
		st.scroll = min(st.scroll+1, maxScroll)
	case "k", "up":
		st.scroll = max(st.scroll-1, 0)
	case "ctrl+d", "pgdown", " ":
		st.scroll = min(st.scroll+h/2, maxScroll)
	case "ctrl+u", "pgup":
		st.scroll = max(st.scroll-h/2, 0)
	case "g", "home":
		st.scroll = 0
	case "G", "end":
		st.scroll = maxScroll
	case "c":
		cmd := m.push(viewChecks)
		return m, cmd, true
	case "d":
		cmd := m.push(viewDiff)
		return m, cmd, true
	case "m":
		next, cmd := m.startMerge()
		return next, cmd, true
	case "M":
		next, cmd := m.startProjectAssign(m.pr)
		return next, cmd, true
	case "C":
		return m.startPRComment(), nil, true
	case "r":
		return m.startPRReview(), nil, true
	case "o":
		if st.d.URL != "" {
			m = m.open(st.d.URL)
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	st.img = visibleSel(doc, st.img, st.scroll, h)
	return m.withScreenData(viewPRDetail, st), nil, true
}

func (m model) contentWidth() int { return max(m.width-2, 20) }

// The detail screens put the sidebar in a column beside the text from this width (of the text) on.
const (
	detailSplitMin  = 100
	detailSideWidth = 32
)

func prDetailBody(m model) string {
	st, _ := m.screenData(viewPRDetail).(prDetailState)
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(m.err.Error()) + "\n")
		h--
	}
	paintDoc(&b, prDetailLines(st.d, m.contentWidth()), st.img, st.scroll, h)
	return b.String()
}

var reviewLabels = map[string]string{
	"APPROVED": "approved", "CHANGES_REQUESTED": "changes requested", "COMMENTED": "reviewed",
	"DISMISSED": "dismissed", "PENDING": "pending review",
}

// prDetailLines lays out the whole detail screen as plain lines at the given width.
func prDetailLines(d ghcli.PRDetail, width int) detailDoc {
	if d.Number == 0 && d.Title == "" {
		return detailDoc{lines: []string{"loading…"}}
	}
	var out detailDoc
	out.add(fmt.Sprintf("#%d  %s", d.Number, d.Title))
	state := d.State
	if d.IsDraft {
		state += " (draft)"
	}
	out.add(fmt.Sprintf("%s   %s ← %s   %s", d.Author, d.BaseRef, d.HeadRef, state))
	out.add(labelValue("Review", orDash(d.ReviewDecision)) + "   " +
		labelValue("Merge", orDash(d.MergeState)) + "   " +
		labelValue("Checks", fmt.Sprintf("%s%d %s%d %s%d", icons.Get(icons.CheckPass), d.ChecksPassed, icons.Get(icons.CheckFail), d.ChecksFailed, icons.Get(icons.CheckPending), d.ChecksPending)) + "   " +
		fmt.Sprintf("+%d −%d in %d files", d.Additions, d.Deletions, d.ChangedFiles))
	split := width >= detailSplitMin
	mainW := width
	if split {
		mainW = width - detailSideWidth - 1
	} else {
		out.add("")
		out.add(prSidebar(d, width)...)
	}
	var body detailDoc // the description and the discussion; beside the sidebar when the screen is wide
	body.add("", "── Description ──")
	if strings.TrimSpace(d.Body) == "" {
		body.add("(no description)")
	} else {
		body.addMarkdown(d.Body, mainW, "")
	}
	body.add("", "── Discussion ──")
	body.addDoc(discussionLines(d.Comments, d.Reviews, mainW))
	if !split {
		out.addDoc(body)
		return out
	}
	// the first row of the sidebar lines up with the description's heading row
	side := append([]string{""}, prSidebarColumn(d, detailSideWidth)...)
	out.addDoc(columns(body, side, mainW))
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// wrap breaks text into lines of at most w columns: at spaces when it can, in the middle of a
// word when a single word is longer than a line. Existing line breaks (LF or CRLF) are kept.
func wrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		flush := func() {
			if line != "" {
				out = append(out, line)
				line = ""
			}
		}
		for _, word := range strings.Fields(para) {
			for utf8.RuneCountInString(word) > w {
				flush()
				r := []rune(word)
				out = append(out, string(r[:w]))
				word = string(r[w:])
			}
			switch {
			case line == "":
				line = word
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= w:
				line += " " + word
			default:
				flush()
				line = word
			}
		}
		flush()
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// discussionLines lists comments and reviews in time order, each under a "who did what · age" line.
func discussionLines(comments []ghcli.Comment, reviews []ghcli.Review, width int) detailDoc {
	type entry struct{ when, head, body string }
	var entries []entry
	for _, c := range comments {
		entries = append(entries, entry{c.CreatedAt, c.Author + " commented", c.Body})
	}
	for _, r := range reviews {
		label := reviewLabels[r.State]
		if label == "" {
			label = strings.ToLower(r.State)
		}
		entries = append(entries, entry{r.CreatedAt, r.Author + " " + label, r.Body})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].when < entries[j].when })
	if len(entries) == 0 {
		return detailDoc{lines: []string{"(no comments)"}}
	}
	var out detailDoc
	for _, e := range entries {
		out.add("", fmt.Sprintf("%s · %s", e.head, ui.Age(e.when)))
		if strings.TrimSpace(e.body) != "" {
			out.addMarkdown(e.body, width, "  ")
		}
	}
	return out
}
