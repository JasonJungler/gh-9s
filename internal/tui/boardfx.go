package tui

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/MurabitoB/gh-9s/internal/anim"
)

// The Fluent-motion experiment on the Project board columns: the selected card fades in as the cursor arrives
// and fades out as it leaves. It needs a truecolor terminal (blending needs real RGB) and is off with
// GH9S_MOTION=off; everywhere else the plain reverse-video selection stays.

const cardFade = 180 * time.Millisecond

type fxTickMsg struct{}

func fxTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(time.Time) tea.Msg { return fxTickMsg{} })
}

// motionOn says whether the card fade is drawn at all.
func motionOn() bool {
	return os.Getenv("GH9S_MOTION") != "off" && lipgloss.ColorProfile() == termenv.TrueColor
}

// cardFx remembers the last selection change: the card the cursor left, the one it reached, and when.
type cardFx struct {
	prev, cur string
	start     time.Time
}

func (fx cardFx) progress(now time.Time) float64 {
	if fx.start.IsZero() {
		return 1
	}
	return anim.Standard(float64(now.Sub(fx.start)) / float64(cardFade))
}

func (fx cardFx) active(now time.Time) bool {
	return !fx.start.IsZero() && now.Sub(fx.start) < cardFade
}

// level is how selected the card with this id looks right now: 0 idle, 1 selected.
func (fx cardFx) level(id string, selected bool, now time.Time) float64 {
	p := fx.progress(now)
	switch {
	case id != "" && id == fx.cur:
		return p
	case id != "" && id == fx.prev && !selected:
		return 1 - p
	case selected:
		return 1
	}
	return 0
}

// fluentCard colours one card line for a selection level; dark and light terminals each get their own palette.
func fluentCard(line string, level float64) string {
	bg0, bg1, fg0, fg1 := "#292929", "#0f3a5f", "#cfcfcf", "#ffffff"
	if !lipgloss.HasDarkBackground() {
		bg0, bg1, fg0, fg1 = "#f3f3f3", "#cfe4fa", "#242424", "#0b2a4a"
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(anim.Mix(bg0, bg1, level))).
		Foreground(lipgloss.Color(anim.Mix(fg0, fg1, level))).
		Bold(level > 0.5).Render(line)
}

// selCardID is the id of the selected card of the columns, "" when there is none.
func selCardID(st boardState, cols []boardCol) string {
	if st.col >= len(cols) || len(cols[st.col].cards) == 0 {
		return ""
	}
	c := cols[st.col].cards
	return st.data.Items[c[min(st.curs[st.col], len(c)-1)]].ID
}

// fadeFrom starts the fade when the selection moved from one card to another.
func (m model) fadeFrom(before, after string) (model, tea.Cmd) {
	if before == after || !motionOn() {
		return m, nil
	}
	m.fx = cardFx{prev: before, cur: after, start: time.Now()}
	return m, fxTick()
}
