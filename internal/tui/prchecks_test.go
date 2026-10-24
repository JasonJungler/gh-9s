package tui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func checksListModel() model {
	return model{view: viewPRs, target: acme, width: 120, height: 20}
}

func listMsg(nums ...int) dataMsg {
	var prs []prListItem
	for _, n := range nums {
		prs = append(prs, prListItem{Number: n, Title: "t"})
	}
	return dataMsg{view: viewPRs, tbl: prRows(prs, "")}
}

func stubChecks(t *testing.T, f func() (map[string]prCheck, error)) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	old := fetchPRChecks
	fetchPRChecks = func(ghcli.RepoRef, ghcli.PRFilter) (map[string]prCheck, error) { calls.Add(1); return f() }
	t.Cleanup(func() { fetchPRChecks = old })
	return &calls
}

func TestPRListArgsLeaveTheHeavyChecksOut(t *testing.T) {
	if strings.Contains(strings.Join(prListArgs(acme, ghcli.PRFilter{}), " "), "statusCheckRollup") {
		t.Error("the list query must not ask for statusCheckRollup")
	}
	if !strings.Contains(strings.Join(prQueryArgs(acme, ghcli.PRFilter{Mine: true}, 50, "number,statusCheckRollup"), " "), "--author") {
		t.Error("the checks query must use the same filter")
	}
}

func TestChecksFillInAfterTheListAndAFailureOnlyLeavesDashes(t *testing.T) {
	calls := stubChecks(t, func() (map[string]prCheck, error) {
		return map[string]prCheck{"1": {"✓ 3/3", stOK}}, nil
	})
	m := checksListModel()
	next, cmd := m.Update(listMsg(1, 2))
	m = next.(model)
	if cmd == nil || m.tbl.rows[0].cells[1] != "…" {
		t.Fatalf("the list shows … while the checks load: %v", m.tbl.rows[0].cells)
	}
	// cmd is a batch: run the fetch directly
	pm := prChecksMsg{key: m.prChecksKey}
	pm.checks, pm.err = fetchPRChecks(acme, ghcli.PRFilter{})
	m = m.prChecksArrived(pm)
	if got := m.tbl.rows[0].cells[1]; got != "✓ 3/3" || m.tbl.rows[0].state != stOK {
		t.Errorf("row 1 = %q %v", got, m.tbl.rows[0].state)
	}
	if got := m.tbl.rows[1].cells[1]; got != "…" && got != "-" {
		t.Errorf("row 2 = %q", got)
	}
	if calls.Load() < 1 {
		t.Error("checks were never asked for")
	}

	// a failing checks request keeps the list and says why
	m = checksListModel()
	next, _ = m.Update(listMsg(1))
	m = next.(model)
	m = m.prChecksArrived(prChecksMsg{key: m.prChecksKey, err: errors.New("HTTP 504")})
	if m.tbl.rows[0].cells[1] != "…" && m.tbl.rows[0].cells[1] != "-" || !strings.Contains(m.status, "504") {
		t.Errorf("failure: cell %q status %q", m.tbl.rows[0].cells[1], m.status)
	}
	if m.prChecksBusy {
		t.Error("a failure must free the request slot")
	}
}

func TestChecksAreNotAskedForOnEveryPoll(t *testing.T) {
	stubChecks(t, func() (map[string]prCheck, error) { return map[string]prCheck{}, nil })
	m := checksListModel()
	m, cmd := m.prChecksAfterList()
	if cmd == nil {
		t.Fatal("first list asks for checks")
	}
	m = m.prChecksArrived(prChecksMsg{key: m.prChecksKey, checks: map[string]prCheck{}})
	if _, cmd = m.prChecksAfterList(); cmd != nil {
		t.Error("a poll within 30s must reuse the checks")
	}
	m.prChecksAt = time.Now().Add(-time.Minute)
	if _, cmd = m.prChecksAfterList(); cmd == nil {
		t.Error("old checks are asked for again")
	}
}

func TestLateChecksForAnotherRepoAreDropped(t *testing.T) {
	m := checksListModel()
	m, _ = m.prChecksAfterList()
	m.tbl = prRows([]prListItem{{Number: 1, Title: "t"}}, "")
	m = m.prChecksArrived(prChecksMsg{key: "other", checks: map[string]prCheck{"1": {"✓", stOK}}})
	if m.tbl.rows[0].cells[1] == "✓" || !m.prChecksBusy {
		t.Error("an answer for another repo or filter must be ignored")
	}
}

func TestRetryOnlyOnGatewayErrors(t *testing.T) {
	n := 0
	err := withRetry(func() error {
		n++
		if n == 1 {
			return errors.New("HTTP 504: 504 Gateway Timeout")
		}
		return nil
	})
	if err != nil || n != 2 {
		t.Errorf("504 is retried once: err=%v n=%d", err, n)
	}
	n = 0
	_ = withRetry(func() error { n++; return errors.New("HTTP 404: Not Found") })
	if n != 1 {
		t.Errorf("404 must not be retried (%d calls)", n)
	}
}
