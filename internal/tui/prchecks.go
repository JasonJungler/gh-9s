package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// The PR list loads without its CHECKS column, which GitHub computes slowly (on a repo with many checks
// per PR it takes seconds or ends in a 504). The column fills in when a separate request answers, and
// stays "-" if that one fails: a slow column never takes the list down with it.

const prChecksEvery = 30 * time.Second // how long the checks are reused when the list is refetched

// prCheck is one PR's CHECKS cell.
type prCheck struct {
	label string
	st    state
}

type prChecksMsg struct {
	key    string
	checks map[string]prCheck
	err    error
}

// fetchPRChecks asks for the checks of the PRs the list shows; tests replace it.
var fetchPRChecks = func(r ghcli.RepoRef, f ghcli.PRFilter) (map[string]prCheck, error) {
	var prs []struct {
		Number       int         `json:"number"`
		StatusRollup []rollupRef `json:"statusCheckRollup"`
	}
	// On a repo whose PRs have many checks even this can time out; then the newest PRs, which are the
	// ones on screen, still get their column.
	var err error
	for _, limit := range []int{50, 20} {
		if err = ghcli.JSON(&prs, prQueryArgs(r, f, limit, "number,statusCheckRollup")...); err == nil || !transient(err) { // the smaller ask is the retry
			break
		}
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]prCheck, len(prs))
	for _, p := range prs {
		label, st := rollupState(p.StatusRollup)
		out[fmt.Sprint(p.Number)] = prCheck{label, st}
	}
	return out, nil
}

// withRetry runs fn, and once more when GitHub answered with a gateway error or a timeout.
func withRetry(fn func() error) error {
	err := fn()
	if err != nil && transient(err) {
		err = fn()
	}
	return err
}

func transient(err error) bool {
	s := strings.ToLower(err.Error())
	for _, w := range []string{"http 502", "http 503", "http 504", "timeout", "timed out", "deadline"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func (m model) prChecksKeyNow() string { return fmt.Sprintf("%v|%+v", m.target, m.prFilter) }

// prChecksAfterList fills the CHECKS cells from what is known and asks for them if they are missing
// or old.
func (m model) prChecksAfterList() (model, tea.Cmd) {
	key := m.prChecksKeyNow()
	if m.prChecksKey != key {
		m.prChecks, m.prChecksKey, m.prChecksAt, m.prChecksBusy = nil, key, time.Time{}, false
	}
	var cmd tea.Cmd
	if !m.prChecksBusy && time.Since(m.prChecksAt) >= prChecksEvery {
		m.prChecksBusy = true
		tg, f := m.target, m.prFilter
		cmd = func() tea.Msg {
			c, err := fetchPRChecks(tg, f)
			return prChecksMsg{key: key, checks: c, err: err}
		}
	}
	return m.applyPRChecks(), cmd
}

// applyPRChecks writes the known checks into the list's rows: "…" while they load, "-" if unknown.
func (m model) applyPRChecks() model {
	rows := make([]row, len(m.tbl.rows))
	copy(rows, m.tbl.rows)
	for i, r := range rows {
		cells := append([]string(nil), r.cells...)
		if len(cells) < 2 {
			continue
		}
		if c, ok := m.prChecks[r.id]; ok {
			cells[1], r.state = c.label, c.st
		} else if m.prChecksBusy {
			cells[1] = "…"
		}
		r.cells = cells
		rows[i] = r
	}
	m.tbl.rows = rows
	return m
}

func (m model) prChecksArrived(msg prChecksMsg) model {
	if msg.key != m.prChecksKey {
		return m // a late answer for another repo or filter
	}
	m.prChecksBusy, m.prChecksAt = false, time.Now() // a failure also waits before the next try
	if msg.err != nil {
		m.status = styDim.Render("checks column unavailable: " + msg.err.Error())
	} else {
		m.prChecks = msg.checks
	}
	if m.view == viewPRs {
		m = m.applyPRChecks()
	}
	return m
}
