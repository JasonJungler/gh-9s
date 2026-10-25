package tui

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/MurabitoB/gh-9s/internal/anim"
)

// Fluent-style motion. Everything here is transient: no row is added, nothing stays on screen once the motion
// is over, so the information density is the one the screens always had.
//
//   - moving between screens pushes (deeper) or pops (back) the body sideways
//   - switching a tab slides the content below the tab strip, and the highlighted tab slides along the strip
//   - the selected row or card fades in as the cursor arrives and fades out as it leaves
//   - a dialog, picker or form rises into place; "loading…" gets a spinner
//
// Sliding works on any terminal; the fades blend colours and need truecolor. GH9S_MOTION=off turns it all off.
const (
	slideTime = 220 * time.Millisecond
	tabTime   = 260 * time.Millisecond
	riseTime  = 160 * time.Millisecond
	fadeTime  = 180 * time.Millisecond
	frameTime = 16 * time.Millisecond
)

// motionNow is the clock; tests replace it.
var motionNow = time.Now

type fxTickMsg struct{}

func fxTick() tea.Cmd {
	return tea.Tick(frameTime, func(time.Time) tea.Msg { return fxTickMsg{} })
}

func motionEnabled() bool { return os.Getenv("GH9S_MOTION") != "off" }

// trueColor says whether colours can be blended; tests replace it.
var trueColor = func() bool { return lipgloss.ColorProfile() == termenv.TrueColor }

func (m model) motionOn() bool    { return m.mo != nil && motionEnabled() }
func (m model) motionColor() bool { return m.motionOn() && trueColor() }

// motionState is shared by every copy of the model: the frames are drawn from a value-typed model, so what has
// to persist between frames lives behind this pointer.
type motionState struct {
	key     sceneKey
	haveKey bool
	shown   string // the last frame drawn, as the screen showed it
	tr      *transition
	strips  map[string]*stripAnim
	sel     map[string]*selFade
	ticking bool
}

func newMotion() *motionState {
	return &motionState{strips: map[string]*stripAnim{}, sel: map[string]*selFade{}}
}

// busy says whether another frame is needed; it also forgets motions that are over.
func (mo *motionState) busy(m model, now time.Time) bool {
	if mo == nil {
		return false
	}
	if mo.tr != nil && now.Sub(mo.tr.start) >= mo.tr.dur {
		mo.tr = nil
	}
	busy := mo.tr != nil || m.loading
	for _, s := range mo.strips {
		busy = busy || !s.x.Done(now) || !s.w.Done(now)
	}
	for _, f := range mo.sel {
		for id, tw := range f.tw {
			if tw.Done(now) {
				delete(f.tw, id)
			} else {
				busy = true
			}
		}
	}
	return busy
}

// View draws the screen and, while a motion runs, the frame between the old screen and the new one.
func (m model) View() string {
	out := m.plainView()
	if !m.motionOn() || m.width <= 0 {
		return out
	}
	return m.mo.compose(m, out)
}

// sceneKey is what makes one screen a different screen for the purpose of sliding.
type sceneKey struct {
	view               viewKind
	depth              int // how many screens are behind this one
	overlay            int // help, dialog, picker, form
	tab, sub           int // the tab and, on the board, the view
	fixedTab, fixedSub int // the lines above and including the strip, which stay put when that strip switches
}

func (m model) sceneKey() sceneKey {
	k := sceneKey{view: m.view, depth: len(m.back)}
	switch {
	case m.helpOpen:
		k.overlay = 1
	case m.dlg != nil:
		k.overlay = 2
	case m.picker != nil:
		k.overlay = 3
	case m.form != nil:
		k.overlay = 4
	}
	switch m.view {
	case viewFavs:
		k.tab, k.fixedTab = m.favTab, 3
	case viewBoard:
		if st := m.boardSt(); st.loaded {
			k.sub, k.fixedSub = st.view, 3
			if !st.colsActive(m) {
				k.tab, k.fixedTab = st.tab, 3+boolToInt(st.hasViews())
			}
		}
	}
	return k
}

type transition struct {
	from  string
	rise  bool // a dialog rising into place instead of a sideways slide
	dir   int  // +1: the new screen comes in from the right
	fixed int  // lines at the top that do not move
	start time.Time
	dur   time.Duration
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

func startTransition(old, cur sceneKey, from string, now time.Time) *transition {
	if from == "" {
		return nil
	}
	switch {
	case cur.overlay != 0 && old.overlay == 0:
		return &transition{from: from, rise: true, fixed: 2, start: now, dur: riseTime}
	case cur.overlay != 0 || old.overlay != 0:
		return nil
	case old.view != cur.view || old.depth != cur.depth:
		return &transition{from: from, dir: sign(cur.depth - old.depth), fixed: 2, start: now, dur: slideTime}
	case old.tab != cur.tab:
		return &transition{from: from, dir: sign(cur.tab - old.tab), fixed: cur.fixedTab, start: now, dur: tabTime}
	case old.sub != cur.sub:
		return &transition{from: from, dir: sign(cur.sub - old.sub), fixed: cur.fixedSub, start: now, dur: tabTime}
	}
	return nil
}

func (mo *motionState) compose(m model, out string) string {
	now := motionNow()
	k := m.sceneKey()
	if mo.haveKey && k != mo.key {
		if tr := startTransition(mo.key, k, mo.shown, now); tr != nil {
			mo.tr = tr
		}
	}
	mo.key, mo.haveKey = k, true
	frame := out
	if mo.tr != nil {
		if now.Sub(mo.tr.start) >= mo.tr.dur {
			mo.tr = nil
		} else {
			frame = mo.tr.apply(out, m.width, now)
		}
	}
	mo.shown = frame
	return frame
}

// apply draws the frame between the old screen and cur at time now.
func (t *transition) apply(cur string, w int, now time.Time) string {
	p := anim.Decelerate(float64(now.Sub(t.start)) / float64(t.dur))
	oldL, newL := strings.Split(t.from, "\n"), strings.Split(cur, "\n")
	n := len(newL)
	if n > 0 && newL[n-1] == "" {
		n-- // the trailing newline is not a line
	}
	if t.rise {
		r := int((1-p)*3 + 0.5)
		for i := n - 1; i >= t.fixed; i-- {
			if src := i - r; src >= t.fixed {
				newL[i] = newL[src]
			} else {
				newL[i] = ""
			}
		}
		return strings.Join(newL, "\n")
	}
	for i := t.fixed; i < n; i++ {
		o := ""
		if i < len(oldL) {
			o = oldL[i]
		}
		left, right, start := o, newL[i], int(p*float64(w)+0.5)
		if t.dir < 0 {
			left, right, start = newL[i], o, int((1-p)*float64(w)+0.5)
		}
		row := padTo(ansi.Truncate(left, w, ""), w) + padTo(ansi.Truncate(right, w, ""), w)
		newL[i] = ansi.Cut(row, start, start+w)
	}
	return strings.Join(newL, "\n")
}

// stripAnim moves the highlighted tab along its strip.
type stripAnim struct {
	x, w     anim.Tween
	toX, toW int
}

// strip says where the highlighted tab of strip id is drawn now, given where it belongs.
func (mo *motionState) strip(id string, x, w int) (cx, cw int, moving bool) {
	now := motionNow()
	s := mo.strips[id]
	if s == nil {
		s = &stripAnim{x: anim.Still(float64(x)), w: anim.Still(float64(w)), toX: x, toW: w}
		mo.strips[id] = s
	}
	if s.toX != x || s.toW != w {
		s.x = s.x.Retarget(float64(x), now, tabTime, anim.Decelerate)
		s.w = s.w.Retarget(float64(w), now, tabTime, anim.Decelerate)
		s.toX, s.toW = x, w
	}
	return int(s.x.At(now) + 0.5), int(s.w.At(now) + 0.5), !s.x.Done(now) || !s.w.Done(now)
}

// tabs draws a tab strip; while the selection changes, the highlight slides from the old tab to the new one.
func (m model) tabs(id string, labels []string, sel int, sep ...string) string {
	line := fitTabs(labels, sel, m.width, sep...)
	if !m.motionOn() || sel < 0 || sel >= len(labels) || m.width <= 0 {
		return line
	}
	s := " "
	if len(sep) > 0 {
		s = sep[0]
	}
	x := 0
	if start := tabStart(labels, sel, m.width, s); start > 0 {
		x = 2 // the "‹ "
		for i := start; i < sel; i++ {
			x += ansi.StringWidth(labels[i]) + ansi.StringWidth(s)
		}
	} else {
		for i := 0; i < sel; i++ {
			x += ansi.StringWidth(labels[i]) + ansi.StringWidth(s)
		}
	}
	cx, cw, moving := m.mo.strip(id, x, ansi.StringWidth(labels[sel]))
	if !moving {
		return line
	}
	plain := ansi.Strip(line)
	total := ansi.StringWidth(plain)
	cx = max(0, min(cx, total))
	end := min(cx+cw, total)
	return styDim.Render(ansi.Cut(plain, 0, cx)) + stySel.Render(ansi.Cut(plain, cx, end)) + styDim.Render(ansi.Cut(plain, end, total))
}

// selFade fades the selected row or card: the level of each item that is on its way to lit or to dark.
type selFade struct {
	cur string
	tw  map[string]anim.Tween
}

// level is how lit the item id looks now: 0 idle, 1 selected. It is called for every item drawn; the one that
// is selected and not yet the current one starts the fades.
func (mo *motionState) level(scope, id string, selected bool, now time.Time) float64 {
	f := mo.sel[scope]
	if f == nil {
		f = &selFade{tw: map[string]anim.Tween{}}
		mo.sel[scope] = f
	}
	if selected && id != f.cur {
		if f.cur != "" {
			old := f.tw[f.cur]
			if _, ok := f.tw[f.cur]; !ok {
				old = anim.Still(1)
			}
			f.tw[f.cur] = old.Retarget(0, now, fadeTime, anim.Standard)
			f.tw[id] = f.tw[id].Retarget(1, now, fadeTime, anim.Standard)
		}
		f.cur = id
	}
	if tw, ok := f.tw[id]; ok {
		return tw.At(now)
	}
	if selected {
		return 1
	}
	return 0
}

// rowLevel is the selection level of table row r, drawn at index i of the visible rows.
func (m model) rowLevel(r row, i int) float64 {
	if !m.motionColor() {
		return 0
	}
	id := r.id
	if id == "" {
		id = "#" + strconv.Itoa(i)
	}
	return m.mo.level("row:"+viewNames[m.view], id, i == m.cursor, motionNow())
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spin is the spinner for "loading…": a frame that turns every 80 ms, nothing with motion off.
func (m model) spin() string {
	if !m.motionOn() {
		return ""
	}
	return spinFrames[int(motionNow().UnixMilli()/80)%len(spinFrames)] + " "
}

// termBackground is the terminal's own background, so a fade starts from exactly what is there; a guess when
// the terminal does not tell.
func termBackground() string {
	if h, ok := bgHex(); ok {
		return h
	}
	if lipgloss.HasDarkBackground() {
		return "#1c1c1c"
	}
	return "#fafafa"
}

var bgCache struct {
	done bool
	hex  string
	ok   bool
}

func bgHex() (string, bool) {
	if !bgCache.done {
		bgCache.done = true
		if c, ok := termenv.BackgroundColor().(termenv.RGBColor); ok {
			bgCache.hex, bgCache.ok = string(c), true
		}
	}
	return bgCache.hex, bgCache.ok
}

// fluentTint colours a line for a selection level between the background bg0 and the selected look.
func fluentTint(line string, level float64, bg0 string) string {
	bg1, fg0, fg1 := "#0f3a5f", "#cfcfcf", "#ffffff"
	if !lipgloss.HasDarkBackground() {
		bg1, fg0, fg1 = "#cfe4fa", "#242424", "#0b2a4a"
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(anim.Mix(bg0, bg1, level))).
		Foreground(lipgloss.Color(anim.Mix(fg0, fg1, level))).
		Bold(level > 0.5).Render(line)
}

// fluentCard colours one card line: a card is a tinted block, a little lighter than the page, that lights up.
func fluentCard(line string, level float64) string {
	bg0 := "#292929"
	if !lipgloss.HasDarkBackground() {
		bg0 = "#f3f3f3"
	}
	return fluentTint(line, level, bg0)
}
