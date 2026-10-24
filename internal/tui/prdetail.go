package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls the PR screens make; tests replace them.
var (
	fetchPR = ghcli.FetchPR
	openURL = ghcli.OpenURL
)

// prDetailState is what the PR detail screen keeps in the model.
type prDetailState struct {
	d      ghcli.PRDetail
	scroll int
}

func init() {
	register(viewPRDetail, screen{
		name: "pr",
		fetch: func(m model) tea.Cmd {
			tg, number := m.target, m.pr
			return func() tea.Msg {
				d, err := fetchPR(tg, number)
				return screenMsg{view: viewPRDetail, data: d, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			old, _ := m.screenData(viewPRDetail).(prDetailState)
			return m.withScreenData(viewPRDetail, prDetailState{d: msg.data.(ghcli.PRDetail), scroll: old.scroll})
		},
		key:    prDetailKey,
		body:   prDetailBody,
		footer: "c checks  d diff  o open  j/k scroll  g/G top/bottom",
	})
}

func prDetailKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewPRDetail).(prDetailState)
	h := m.logHeight()
	maxScroll := max(0, len(prDetailLines(st.d, m.contentWidth()))-h)
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
	case "o":
		if st.d.URL != "" {
			_ = openURL(st.d.URL)
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	return m.withScreenData(viewPRDetail, st), nil, true
}

func (m model) contentWidth() int { return max(m.width-2, 20) }

func prDetailBody(m model) string {
	st, _ := m.screenData(viewPRDetail).(prDetailState)
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(m.err.Error()) + "\n")
		h--
	}
	lines := prDetailLines(st.d, m.contentWidth())
	n := 0
	for i := min(st.scroll, max(0, len(lines)-1)); i < len(lines) && n < h; i++ {
		b.WriteString(" " + lines[i] + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

var reviewLabels = map[string]string{
	"APPROVED": "approved", "CHANGES_REQUESTED": "changes requested", "COMMENTED": "reviewed",
	"DISMISSED": "dismissed", "PENDING": "pending review",
}

// prDetailLines lays out the whole detail screen as plain lines at the given width.
func prDetailLines(d ghcli.PRDetail, width int) []string {
	if d.Number == 0 && d.Title == "" {
		return []string{"loading…"}
	}
	var out []string
	out = append(out, fmt.Sprintf("#%d  %s", d.Number, d.Title))
	state := d.State
	if d.IsDraft {
		state += " (draft)"
	}
	out = append(out, fmt.Sprintf("%s   %s ← %s   %s", d.Author, d.BaseRef, d.HeadRef, state))
	out = append(out, fmt.Sprintf("Review: %s   Merge: %s   Checks: ✓%d ✗%d ●%d   +%d −%d in %d files",
		orDash(d.ReviewDecision), orDash(d.MergeState), d.ChecksPassed, d.ChecksFailed, d.ChecksPending,
		d.Additions, d.Deletions, d.ChangedFiles))
	if len(d.Labels) > 0 {
		out = append(out, "Labels: "+strings.Join(d.Labels, ", "))
	}
	out = append(out, "", "── Description ──")
	if strings.TrimSpace(d.Body) == "" {
		out = append(out, "(no description)")
	} else {
		out = append(out, wrap(d.Body, width)...)
	}
	out = append(out, "", "── Discussion ──")

	type entry struct{ when, head, body string }
	var entries []entry
	for _, c := range d.Comments {
		entries = append(entries, entry{c.CreatedAt, c.Author + " commented", c.Body})
	}
	for _, r := range d.Reviews {
		label := reviewLabels[r.State]
		if label == "" {
			label = strings.ToLower(r.State)
		}
		entries = append(entries, entry{r.CreatedAt, r.Author + " " + label, r.Body})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].when < entries[j].when })
	if len(entries) == 0 {
		out = append(out, "(no comments)")
	}
	for _, e := range entries {
		out = append(out, "", fmt.Sprintf("%s · %s", e.head, ui.Age(e.when)))
		if strings.TrimSpace(e.body) != "" {
			for _, l := range wrap(e.body, width-2) {
				out = append(out, "  "+l)
			}
		}
	}
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
