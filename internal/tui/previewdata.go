package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// What the preview pane shows is loaded with the same detail fetches the PR and issue screens use,
// after the selection has rested for previewDelay, and kept in a small cache. Tests replace the clock
// and the delay.
var (
	previewNow   = time.Now
	previewDelay = 250 * time.Millisecond
)

// inflightFor is how long a fetch that has not answered still counts as under way: gh can hang, and
// a row must not stay "loading…" for the rest of the session.
const inflightFor = 30 * time.Second

const (
	previewCap    = 50              // entries kept; the least recently used goes first
	previewTTL    = 5 * time.Minute // how long a loaded detail is reused
	previewErrTTL = 30 * time.Second
)

// previewKey says which PR or issue an entry is of: two hosts' acme/api#1 are different.
type previewKey struct {
	host, repo, kind string // kind is "pr" or "issue"
	number           int
}

type previewEntry struct {
	pr         *ghcli.PRDetail
	issue      *ghcli.IssueDetail
	err        error
	at         time.Time
	rowUpdated string // the list row's updatedAt when this was fetched
}

// previewState is shared by every copy of the model (it is only touched on the UI goroutine).
type previewState struct {
	sel      previewKey // the row the pane follows
	has      bool
	seq      int // changes with every selection, so an older tick can tell it is old
	cache    map[previewKey]previewEntry
	order    []previewKey             // least recently used first
	inflight map[previewKey]time.Time // fetches under way, and since when
	ticking  map[previewKey]bool      // a tick is on its way
	top      int                      // the pane's scroll
	paneW    int                      // the size the pane was last drawn at, which scrolling needs
	paneH    int

	lines    []string // the selected entry laid out, kept until the entry or the width changes
	linesKey previewKey
	linesAt  time.Time
	linesW   int
}

func newPreviewState() *previewState {
	return &previewState{cache: map[previewKey]previewEntry{}, inflight: map[previewKey]time.Time{}, ticking: map[previewKey]bool{}}
}

type previewTickMsg struct {
	seq     int
	key     previewKey
	target  ghcli.RepoRef
	updated string
}

type previewLoadedMsg struct {
	key   previewKey
	entry previewEntry
}

func (st *previewState) touch(k previewKey) {
	for i, o := range st.order {
		if o == k {
			st.order = append(st.order[:i], st.order[i+1:]...)
			break
		}
	}
	st.order = append(st.order, k)
}

func (st *previewState) put(k previewKey, e previewEntry) {
	st.cache[k] = e
	st.touch(k)
	for len(st.order) > previewCap {
		delete(st.cache, st.order[0])
		st.order = st.order[1:]
	}
}

// get returns an entry and counts it as used.
func (st *previewState) get(k previewKey) (previewEntry, bool) {
	e, ok := st.cache[k]
	if ok {
		st.touch(k)
	}
	return e, ok
}

// fresh says whether an entry can still be shown without asking again.
func (e previewEntry) fresh(rowUpdated string, now time.Time) bool {
	ttl := previewTTL
	if e.err != nil {
		ttl = previewErrTTL
	}
	return now.Sub(e.at) < ttl && rowUpdated <= e.rowUpdated
}

func normHost(h string) string {
	if h == "" {
		return "github.com"
	}
	return h
}

// previewTarget is what the selected row stands for: its key, the repo to fetch it from and when it was
// last updated. ok is false when no row is selected or the screen has no pane.
func (m model) previewTarget() (key previewKey, tg ghcli.RepoRef, updated string, ok bool) {
	if !m.previewShown() {
		return
	}
	r, sel := m.selected()
	if !sel {
		return
	}
	switch m.view {
	case viewPRs, viewIssues:
		n, err := strconv.Atoi(strings.TrimPrefix(r.id, "*"))
		if err != nil {
			return
		}
		tg = m.curTarget()
		kind := "pr"
		if m.view == viewIssues {
			kind = "issue"
		}
		return previewKey{normHost(tg.Host), tg.Owner + "/" + tg.Name, kind, n}, tg, r.updated, true
	case viewBoard:
		st := m.boardSt()
		it, found := boardItem(st.data, r.id)
		owner, name, cut := strings.Cut(it.Repo, "/")
		if !found || !cut || owner == "" || name == "" || (it.Type != "Issue" && it.Type != "PullRequest") {
			return
		}
		host := m.board.Host
		if host == "github.com" {
			host = ""
		}
		kind := "issue"
		if it.Type == "PullRequest" {
			kind = "pr"
		}
		return previewKey{normHost(host), it.Repo, kind, it.Number}, ghcli.RepoRef{Host: host, Owner: owner, Name: name}, it.Updated, true
	case viewInbox:
		st, _ := m.screenData(viewInbox).(inboxState)
		for _, it := range st.tabs[st.tab].items {
			if inboxKey(it) != r.id {
				continue
			}
			owner, name, found := strings.Cut(it.Repo, "/")
			if !found || owner == "" || name == "" {
				return
			}
			host := it.Host
			if host == "github.com" {
				host = ""
			}
			return previewKey{normHost(it.Host), it.Repo, it.Kind, it.Number}, ghcli.RepoRef{Host: host, Owner: owner, Name: name}, it.UpdatedAt, true
		}
	}
	return
}

// previewSync points the pane at the selected row. A row whose detail is not cached (or is old) is
// fetched once the selection has rested for previewDelay; moving on first makes that tick a no-op.
func (m model) previewSync() (model, tea.Cmd) {
	if m.preview == nil {
		m.preview = newPreviewState()
	}
	st := m.preview
	key, tg, updated, ok := m.previewTarget()
	if !ok {
		if st.has {
			st.has, st.sel = false, previewKey{}
			st.seq++ // a tick still on its way finds the selection gone
		}
		return m, nil
	}
	same := st.has && st.sel == key
	if !same {
		st.sel, st.has, st.top = key, true, 0
		st.seq++
	}
	if e, ok := st.get(key); ok && e.fresh(updated, previewNow()) {
		return m, nil
	}
	if st.inflightNow(key) || same && st.ticking[key] {
		return m, nil
	}
	// not loaded, or loaded long ago, or the list says the item changed: fetch after a pause. Staying
	// on the row keeps showing the old text meanwhile.
	st.ticking[key] = true
	seq := st.seq
	return m, tea.Tick(previewDelay, func(time.Time) tea.Msg {
		return previewTickMsg{seq: seq, key: key, target: tg, updated: updated}
	})
}

// inflightNow says whether a fetch for k is under way (one that has not answered for inflightFor is
// given up on).
func (st *previewState) inflightNow(k previewKey) bool {
	since, ok := st.inflight[k]
	return ok && previewNow().Sub(since) < inflightFor
}

func (m model) previewTick(msg previewTickMsg) (tea.Model, tea.Cmd) {
	st := m.preview
	if st == nil {
		return m, nil
	}
	delete(st.ticking, msg.key)
	if !st.has || st.seq != msg.seq || st.sel != msg.key || st.inflightNow(msg.key) {
		return m, nil
	}
	if e, ok := st.cache[msg.key]; ok && e.fresh(msg.updated, previewNow()) {
		return m, nil // it arrived while the tick was on its way
	}
	st.inflight[msg.key] = previewNow()
	return m, func() tea.Msg {
		e := previewEntry{rowUpdated: msg.updated}
		num := fmt.Sprint(msg.key.number)
		if msg.key.kind == "pr" {
			d, err := fetchPR(msg.target, num)
			e.err = err
			if err == nil {
				e.pr = &d
			}
		} else {
			d, err := fetchIssue(msg.target, num)
			e.err = err
			if err == nil {
				e.issue = &d
			}
		}
		return previewLoadedMsg{key: msg.key, entry: e}
	}
}

func (m model) previewLoaded(msg previewLoadedMsg) model {
	if m.preview == nil {
		return m
	}
	delete(m.preview.inflight, msg.key)
	msg.entry.at = previewNow()
	if m.preview.has && m.preview.sel == msg.key {
		m.preview.top = 0 // what the pane holds changed: start at its top
	}
	m.preview.put(msg.key, msg.entry)
	return m
}

// previewCurrent is the loaded entry of the selected row, if there is one.
func (m model) previewCurrent() (previewEntry, bool) {
	if m.preview == nil || !m.preview.has {
		return previewEntry{}, false
	}
	e, ok := m.preview.cache[m.preview.sel]
	return e, ok
}
