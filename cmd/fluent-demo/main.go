// Command fluent-demo is an experiment: a Fluent-style tab strip whose content slides and whose cards fade when
// selected, to see how much of Fluent's motion a terminal can carry. It is not part of gh-9s yet.
//
//	go run ./cmd/fluent-demo      h/l or ←/→ switch tab, j/k or ↑/↓ pick a card, q quits
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/MurabitoB/gh-9s/internal/anim"
)

const (
	width     = 64 // the content area, in cells
	cardH     = 3
	frame     = 16 * time.Millisecond
	slideTime = 280 * time.Millisecond
	fadeTime  = 180 * time.Millisecond
)

// Fluent-ish dark palette.
const (
	colBg       = "#1f1f1f"
	colCard     = "#292929"
	colCardSel  = "#0f3a5f"
	colBorder   = "#454545"
	colAccent   = "#479ef5"
	colText     = "#d6d6d6"
	colTextSel  = "#ffffff"
	colTabIdle  = "#9a9a9a"
	colTabSel   = "#ffffff"
	colTabTrack = "#3a3a3a"
)

var tabs = []struct {
	name  string
	cards [][2]string // title, subtitle
}{
	{"Workflows", [][2]string{
		{"CI", "runs on push and pull request"},
		{"Release", "tags v* · builds the binaries"},
		{"Docs", "deploys the website"},
		{"Graph demo", "records the job graph GIF"},
	}},
	{"Repos", [][2]string{
		{"acme/app", "the main product"},
		{"acme/infra", "terraform and runbooks"},
		{"acme/website", "marketing site"},
		{"MurabitoB/gh-9s", "a k9s-style TUI for GitHub"},
	}},
	{"Projects", [][2]string{
		{"Roadmap", "12 items · 4 in progress"},
		{"Bugs", "7 items · 2 blocked"},
		{"Design", "5 items · all in review"},
		{"Ideas", "20 items · not triaged"},
	}},
}

type tickMsg time.Time

type model struct {
	tab, prev int
	dir       int // +1 when the new tab is to the right of the old one
	slide     anim.Tween
	indX      anim.Tween // left edge of the tab underline
	indW      anim.Tween
	sel       [3]int
	hi        [3][4]anim.Tween // 0..1 highlight per card
	now       time.Time
}

func newModel() model {
	m := model{now: time.Now(), slide: anim.Still(1)}
	x, w := tabSpan(0)
	m.indX, m.indW = anim.Still(float64(x)), anim.Still(float64(w))
	for t := range m.hi {
		for c := range m.hi[t] {
			m.hi[t][c] = anim.Still(0)
		}
		m.hi[t][0] = anim.Still(1) // every tab opens on its first card
	}
	return m
}

// tabSpan is where the tab's label sits on the strip: each tab is padded by two cells either side.
func tabSpan(i int) (x, w int) {
	for j := 0; j < i; j++ {
		x += len(tabs[j].name) + 4
	}
	return x, len(tabs[i].name) + 4
}

func (m model) Init() tea.Cmd { return nil }

func tick() tea.Cmd { return tea.Tick(frame, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m model) animating() bool {
	if !m.slide.Done(m.now) || !m.indX.Done(m.now) || !m.indW.Done(m.now) {
		return true
	}
	for _, row := range m.hi {
		for _, h := range row {
			if !h.Done(m.now) {
				return true
			}
		}
	}
	return false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		if m.animating() {
			return m, tick()
		}
		return m, nil
	case tea.KeyMsg:
		m.now = time.Now()
		wasAnimating := m.animating()
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "l", "right":
			m = m.goTab(m.tab + 1)
		case "h", "left":
			m = m.goTab(m.tab - 1)
		case "j", "down":
			m = m.goCard(m.sel[m.tab] + 1)
		case "k", "up":
			m = m.goCard(m.sel[m.tab] - 1)
		}
		if !wasAnimating && m.animating() {
			return m, tick()
		}
	}
	return m, nil
}

func (m model) goTab(to int) model {
	if to < 0 || to >= len(tabs) || to == m.tab {
		return m
	}
	// A slide interrupted half-way restarts from the old tab's rest position; the underline alone turns around smoothly.
	m.prev, m.dir = m.tab, 1
	if to < m.tab {
		m.dir = -1
	}
	m.tab = to
	m.slide = anim.To(0, 1, m.now, slideTime, anim.Decelerate)
	x, w := tabSpan(to)
	m.indX = m.indX.Retarget(float64(x), m.now, slideTime, anim.Decelerate)
	m.indW = m.indW.Retarget(float64(w), m.now, slideTime, anim.Decelerate)
	return m
}

func (m model) goCard(to int) model {
	n := len(tabs[m.tab].cards)
	if to < 0 || to >= n || to == m.sel[m.tab] {
		return m
	}
	old := m.sel[m.tab]
	m.sel[m.tab] = to
	m.hi[m.tab][old] = m.hi[m.tab][old].Retarget(0, m.now, fadeTime, anim.Standard)
	m.hi[m.tab][to] = m.hi[m.tab][to].Retarget(1, m.now, fadeTime, anim.Standard)
	return m
}

func fg(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

// strip draws the tab names and, under them, the underline that slides between tabs.
func (m model) strip() string {
	var names strings.Builder
	for i, t := range tabs {
		c := colTabIdle
		if i == m.tab {
			c = colTabSel
		}
		names.WriteString(fg(c).Bold(i == m.tab).Render("  " + t.name + "  "))
	}
	x := int(m.indX.At(m.now) + 0.5)
	w := int(m.indW.At(m.now) + 0.5)
	track := []rune(strings.Repeat("─", width))
	for i := x; i < x+w && i < width; i++ {
		track[i] = '━'
	}
	var line strings.Builder
	for i, r := range track {
		c := colTabTrack
		if i >= x && i < x+w {
			c = colAccent
		}
		line.WriteString(fg(c).Render(string(r)))
	}
	return names.String() + "\n" + line.String()
}

// panel draws one tab's cards.
func (m model) panel(t int) string {
	var out []string
	for i, c := range tabs[t].cards {
		p := m.hi[t][i].At(m.now)
		bg := anim.Mix(colCard, colCardSel, p)
		border := anim.Mix(colBorder, colAccent, p)
		title := anim.Mix(colText, colTextSel, p)
		sub := anim.Mix(colTabIdle, colText, p)
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(border)).BorderBackground(lipgloss.Color(colBg)).
			Background(lipgloss.Color(bg)).Width(width-2).Padding(0, 1)
		body := fg(title).Background(lipgloss.Color(bg)).Bold(true).Render(c[0]) + "\n" +
			fg(sub).Background(lipgloss.Color(bg)).Render(c[1])
		out = append(out, box.Render(body))
	}
	return strings.Join(out, "\n")
}

// content is the viewport onto the panel strip: while sliding, old and new panels sit side by side and the
// window moves across them.
func (m model) content() string {
	p := m.slide.At(m.now)
	if m.slide.Done(m.now) {
		return m.panel(m.tab)
	}
	old, cur := m.panel(m.prev), m.panel(m.tab)
	left, right, start := old, cur, int(p*float64(width)+0.5)
	if m.dir < 0 {
		left, right, start = cur, old, int((1-p)*float64(width)+0.5)
	}
	ls, rs := strings.Split(left, "\n"), strings.Split(right, "\n")
	pad := lipgloss.NewStyle().Width(width)
	var rows []string
	for i := range ls {
		row := pad.Render(ls[i]) + pad.Render(rs[i])
		rows = append(rows, ansi.Cut(row, start, start+width))
	}
	return strings.Join(rows, "\n")
}

func (m model) View() string {
	help := fg(colTabIdle).Render("h/l tab · j/k card · q quit")
	return "\n" + m.strip() + "\n\n" + m.content() + "\n\n" + help + "\n"
}

func main() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fluent-demo:", err)
		os.Exit(1)
	}
}
