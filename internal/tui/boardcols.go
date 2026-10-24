package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

const (
	colMinWidth   = 24 // narrowest a column is drawn
	colsMinScreen = 60 // narrower terminals keep the list
	cardRows      = 5  // four lines and a blank one
)

// colsActive says whether the board is drawn as columns right now.
func (st boardState) colsActive(m model) bool {
	return st.cols && m.width >= colsMinScreen && st.loaded
}

// boardCol is one column of the columns layout: a Status of the chosen view, with the cards that pass the filter.
type boardCol struct {
	name  string
	count int   // cards in the column before the typed filter
	cards []int // indices into the Project's items
}

// boardColumns are the Status tabs of the chosen view (All is not a column unless it is the only tab).
func (m model) boardColumns(st boardState) []boardCol {
	idx, _ := st.viewItems()
	tabs := boardTabs(st.data, idx)
	if len(tabs) > 1 {
		tabs = tabs[1:]
	}
	var out []boardCol
	for _, t := range tabs {
		c := boardCol{name: t.name, count: len(t.items)}
		for _, i := range t.items {
			r := boardRows(st.data, boardTab{items: []int{i}})[0]
			if m.filter == "" || strings.Contains(strings.ToLower(strings.Join(r.cells, " ")), strings.ToLower(m.filter)) {
				c.cards = append(c.cards, i)
			}
		}
		out = append(out, c)
	}
	return out
}

func setIdx(m map[int]int, k, v int) map[int]int {
	out := make(map[int]int, len(m)+1)
	for a, b := range m {
		out[a] = b
	}
	out[k] = v
	return out
}

// colFit clamps the selected column and card, and scrolls so that both stay in view.
func (m model) colFit(st boardState, cols []boardCol) boardState {
	if len(cols) == 0 {
		return st
	}
	st.col = min(max(st.col, 0), len(cols)-1)
	n := len(cols[st.col].cards)
	cur := min(max(st.curs[st.col], 0), max(n-1, 0))
	st.curs = setIdx(st.curs, st.col, cur)
	vis := m.colVisibleCards()
	top := st.tops[st.col]
	if cur < top {
		top = cur
	}
	if cur >= top+vis {
		top = cur - vis + 1
	}
	st.tops = setIdx(st.tops, st.col, max(top, 0))
	k, _ := m.colWindow(len(cols))
	if st.col < st.win {
		st.win = st.col
	}
	if st.col >= st.win+k {
		st.win = st.col - k + 1
	}
	st.win = min(max(st.win, 0), max(len(cols)-k, 0))
	return st
}

func (m model) colVisibleCards() int { return max(1, (m.bodyHeight()+1)/cardRows) }

// colWindow is how many columns fit and how wide each one is.
func (m model) colWindow(n int) (k, w int) {
	k = min(n, max(1, (m.width+1)/(colMinWidth+1)))
	if k < 1 {
		k = 1
	}
	return k, max((m.width-(k-1))/k, 1)
}

// boardSelectedRow is the row the cursor is on, in either layout.
func (m model) boardSelectedRow() (row, bool) {
	st := m.boardSt()
	if !st.colsActive(m) {
		return m.selected()
	}
	cols := m.boardColumns(st)
	if st.col >= len(cols) || len(cols[st.col].cards) == 0 {
		return row{}, false
	}
	i := cols[st.col].cards[min(st.curs[st.col], len(cols[st.col].cards)-1)]
	return boardRows(st.data, boardTab{items: []int{i}})[0], true
}

func padCell(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// cardLines draws one card in w columns: the item, the title on up to two lines, and who and what.
func cardLines(p ghcli.Project, i, w int, selected bool) []string {
	it := p.Items[i]
	r := boardRows(p, boardTab{items: []int{i}})[0]
	var tl []string
	if strings.TrimSpace(it.Title) != "" {
		tl = strings.Split(ansi.Wrap(it.Title, w, ""), "\n")
	}
	if len(tl) > 2 {
		tl = []string{tl[0], ansi.Truncate(strings.Join(tl[1:], " "), w, "…")}
	}
	for len(tl) < 2 {
		tl = append(tl, "")
	}
	meta := strings.Join(nonEmpty(it.Assignees), ", ")
	if l := strings.Join(nonEmpty(it.Labels), ","); l != "" {
		if meta != "" {
			meta += " · "
		}
		meta += l
	}
	if meta == "" {
		meta = "-"
	}
	lines := []string{padCell(r.cells[0], w), padCell(tl[0], w), padCell(tl[1], w), padCell(meta, w)}
	if selected {
		for k := range lines {
			lines[k] = ui.Sel.Render(lines[k])
		}
	}
	return lines
}

func nonEmpty(l []string) []string {
	var out []string
	for _, s := range l {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// boardColumnsBody draws the header, the separator and the cards of the visible columns.
func boardColumnsBody(m model, st boardState) string {
	cols := m.boardColumns(st)
	var b strings.Builder
	if st.hasViews() {
		b.WriteString(fitTabs(st.viewLabels(), st.view, m.width) + "\n")
	}
	if len(cols) == 0 {
		return b.String() + "\n"
	}
	st = m.colFit(st, cols)
	k, w := m.colWindow(len(cols))
	first := st.win
	var hdr, sep []string
	var body [][]string
	h := m.bodyHeight()
	for c := first; c < first+k && c < len(cols); c++ {
		col := cols[c]
		name := fmt.Sprintf(" %s (%s)", col.name, boardCount(st.data, col.count))
		cell := padCell(name, w)
		if c == st.col {
			cell = ui.Sel.Render(cell)
		} else {
			cell = styDim.Render(cell)
		}
		hdr = append(hdr, cell)
		sep = append(sep, styDim.Render(strings.Repeat("─", w)))
		var lines []string
		if len(col.cards) == 0 {
			lines = append(lines, styDim.Render(padCell(" (empty)", w)))
		}
		for ci := st.tops[c]; ci < len(col.cards) && len(lines) < h; ci++ {
			lines = append(lines, cardLines(st.data, col.cards[ci], w, c == st.col && ci == st.curs[c])...)
			lines = append(lines, strings.Repeat(" ", w))
		}
		body = append(body, lines)
	}
	head := strings.Join(hdr, " ")
	if first > 0 {
		head = "‹" + ansi.Cut(head, 1, ansi.StringWidth(head))
	}
	if first+k < len(cols) {
		head = ansi.Truncate(head, max(ansi.StringWidth(head)-1, 0), "") + "›"
	}
	b.WriteString(ansi.Truncate(head, m.width, "") + "\n")
	b.WriteString(strings.Join(sep, " ") + "\n")
	for r := 0; r < h; r++ {
		var cells []string
		for _, lines := range body {
			if r < len(lines) {
				cells = append(cells, lines[r])
			} else {
				cells = append(cells, strings.Repeat(" ", w))
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, " "), " ") + "\n")
	}
	_, note := st.viewItems()
	switch {
	case note != "":
		b.WriteString(styErr.Render(" "+trunc(note, max(m.width-2, 10))) + "\n")
	case st.data.Total > len(st.data.Items):
		b.WriteString(styDim.Render(fmt.Sprintf(" %d more in the Project (the first %d are shown)", st.data.Total-len(st.data.Items), len(st.data.Items))) + "\n")
	default:
		b.WriteString("\n")
	}
	return b.String()
}

// toggleBoardColumns is b: switch between the list and the columns, keeping the selected card.
func (m model) toggleBoardColumns() model {
	st := m.boardSt()
	if !st.loaded {
		return m
	}
	if st.cols {
		id := ""
		if r, ok := m.boardSelectedRow(); ok {
			id = r.id
		}
		cols := m.boardColumns(st)
		st.cols = false
		st.tab = 0
		if len(boardTabsFor(st)) > 1 && st.col < len(cols) {
			st.tab = st.col + 1
		}
		return m.rebuildBoard(st, id)
	}
	if m.width < colsMinScreen {
		m.status = styDim.Render("columns need at least 60 columns of width")
		return m
	}
	id := ""
	if r, ok := m.selected(); ok {
		id = r.id
	}
	st.cols = true
	st.col, st.curs, st.tops, st.win = max(st.tab-1, 0), nil, nil, 0
	cols := m.boardColumns(st)
	for c, col := range cols { // find the card of the list's cursor
		for ci, i := range col.cards {
			if id != "" && boardRowID(st.data, i) == id {
				st.col = c
				st.curs = setIdx(st.curs, c, ci)
			}
		}
	}
	m = m.withScreenData(viewBoard, m.colFit(st, cols))
	return m
}

func boardTabsFor(st boardState) []boardTab {
	idx, _ := st.viewItems()
	return boardTabs(st.data, idx)
}

// boardColsKey handles the movement keys of the columns layout.
func boardColsKey(m model, st boardState, k tea.KeyMsg) (model, tea.Cmd, bool) {
	cols := m.boardColumns(st)
	moveCol := func(d int) (model, tea.Cmd, bool) {
		if len(cols) > 0 {
			st.col = (st.col + d + len(cols)) % len(cols)
		}
		m = m.withScreenData(viewBoard, m.colFit(st, cols))
		return m, nil, true
	}
	moveCard := func(d int) (model, tea.Cmd, bool) {
		if len(cols) > 0 && st.col < len(cols) {
			st.curs = setIdx(st.curs, st.col, min(max(st.curs[st.col]+d, 0), max(len(cols[st.col].cards)-1, 0)))
		}
		m = m.withScreenData(viewBoard, m.colFit(st, cols))
		return m, nil, true
	}
	switch k.String() {
	case "l", "right", "tab":
		return moveCol(1)
	case "h", "left", "shift+tab":
		return moveCol(-1)
	case "j", "down":
		return moveCard(1)
	case "k", "up":
		return moveCard(-1)
	case "g", "home":
		return moveCard(-1 << 20)
	case "G", "end":
		return moveCard(1 << 20)
	case "ctrl+d", "pgdown":
		return moveCard(m.colVisibleCards())
	case "ctrl+u", "pgup":
		return moveCard(-m.colVisibleCards())
	}
	return m, nil, false
}
