package tui

// target is where an inline comment goes: a file, a line number on one side of the diff.
type target struct {
	Path string
	Line int
	Side string // "LEFT" (old file) or "RIGHT" (new file)
}

// rowTarget says what a comment on display row `row` would be about, in whichever layout is shown.
// oldSide asks for the old file's line (a deleted line, or a context line seen from the old side).
// When the row cannot take a comment the second result says why.
func rowTarget(st diffState, row int, oldSide bool) (target, string) {
	if len(st.d.Files) == 0 {
		return target{}, "nothing to comment on"
	}
	var path string
	var oldNo, newNo int
	if st.sb != nil {
		if row < 0 || row >= len(st.sb.lines) {
			return target{}, "no line here"
		}
		l := st.sb.lines[row]
		switch {
		case l.thread:
			return target{}, "that is a comment, not a code line"
		case l.hunk:
			return target{}, "hunk headers can't be commented on"
		case l.kind == dFile:
			return target{}, "pick a line of code, not the file header"
		case l.oldNo == 0 && l.newNo == 0:
			return target{}, "not a code line"
		}
		path, oldNo, newNo = st.d.Files[fileAt(st.sb.starts, row)].Path, l.oldNo, l.newNo
	} else {
		if row < 0 || row >= len(st.lines) || row >= len(st.usrc) {
			return target{}, "no line here"
		}
		switch st.lines[row].kind {
		case dHunk:
			return target{}, "hunk headers can't be commented on"
		case dFile:
			return target{}, "pick a line of code, not the file header"
		case dAdd, dDel, dCtx:
		default:
			return target{}, "not a code line"
		}
		file, idx := st.usrc[row]>>20, st.usrc[row]&(1<<20-1)-1
		if file >= len(st.d.Files) || idx < 0 {
			return target{}, "not a code line"
		}
		nums := st.d.Files[file].Numbered()
		if idx >= len(nums) {
			return target{}, "not a code line"
		}
		path, oldNo, newNo = st.d.Files[file].Path, nums[idx].Old, nums[idx].New
	}
	switch {
	case oldSide && oldNo > 0:
		return target{path, oldNo, "LEFT"}, ""
	case oldSide:
		return target{}, "this line has no old version"
	case newNo > 0:
		return target{path, newNo, "RIGHT"}, ""
	case oldNo > 0:
		return target{path, oldNo, "LEFT"}, ""
	}
	return target{}, "not a code line"
}

// rowKey is the source key of display row `row` in the layout shown (see srcKey).
func rowKey(st diffState, row int) int {
	switch {
	case st.sb != nil && row >= 0 && row < len(st.sb.src):
		return st.sb.src[row]
	case st.sb == nil && row >= 0 && row < len(st.usrc):
		return st.usrc[row]
	}
	return 0
}

// rowOfKey is the display row that shows source line key in the layout shown.
func rowOfKey(st diffState, key int) int {
	if st.sb != nil {
		return st.sb.rowFor(key)
	}
	return firstAtLeast(st.usrc, key)
}

// setCursor puts the cursor on a row and remembers the source line it is on.
func setCursor(st diffState, row int) diffState {
	st.cur = row
	st.curKey = rowKey(st, row)
	return st
}

// moveCursor handles the keys that walk the cursor: it moves first, and the view scrolls only when
// the cursor would leave it. ok is false for other keys.
func moveCursor(m model, st diffState, key string) (model, diffState, bool) {
	n, h := len(m.logs), m.logHeight()
	if n == 0 {
		return m, st, false
	}
	var to int
	switch key {
	case "j", "down":
		to = st.cur + 1
	case "k", "up":
		to = st.cur - 1
	case "ctrl+d":
		to = st.cur + h/2
	case "ctrl+u":
		to = st.cur - h/2
	case "pgdown", " ":
		to = st.cur + h
	case "pgup":
		to = st.cur - h
	case "g", "home":
		to = 0
	case "G", "end":
		to = n - 1
	default:
		return m, st, false
	}
	to = min(max(to, 0), n-1)
	switch {
	case to < m.top:
		m.top = to
	case to >= m.top+h:
		m.top = to - h + 1
	}
	m.top = min(max(m.top, 0), m.maxTop())
	return m, setCursor(st, to), true
}

// keepCursorVisible drags the cursor onto the screen after something else moved the view (a jump,
// a search, a refetch).
func (m model) keepCursorVisible() model {
	st, _ := m.screenData(viewDiff).(diffState)
	if len(st.d.Files) == 0 || len(m.logs) == 0 {
		return m
	}
	h := m.logHeight()
	if st.cur >= m.top && st.cur < m.top+h && st.cur < len(m.logs) {
		return m
	}
	return m.withScreenData(viewDiff, setCursor(st, min(m.top, len(m.logs)-1)))
}
