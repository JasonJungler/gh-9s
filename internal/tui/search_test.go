package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSearchSmartCase(t *testing.T) {
	lines := []string{"build ok", "Error: boom", "an error here", "ERROR LOUD"}
	if got := searchMatches(lines, "error"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("lowercase query should ignore case: %v", got)
	}
	if got := searchMatches(lines, "Error"); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("query with a capital should match case: %v", got)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	if got := searchMatches([]string{"a"}, ""); got != nil {
		t.Errorf("empty query = %v; want nil", got)
	}
}

func TestSearchNoMatch(t *testing.T) {
	if got := searchMatches([]string{"a", "b"}, "zzz"); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func logModel() model {
	var logs []string
	for i := 0; i < 100; i++ {
		l := fmt.Sprintf("step line %d", i)
		switch i {
		case 10, 50, 90:
			l += " error"
		case 70:
			l += " Error"
		}
		logs = append(logs, l)
	}
	return model{view: viewLogs, width: 100, height: 15, logs: logs, runID: "1", jobName: "build", back: []viewKind{viewGraph}}
}

func TestLogSearchJumpsAndWraps(t *testing.T) {
	m := keys(logModel(), "/", "e", "r", "r", "o", "r", "enter")
	if !reflect.DeepEqual(m.matches, []int{10, 50, 70, 90}) || m.mi != 0 {
		t.Fatalf("matches=%v mi=%d", m.matches, m.mi)
	}
	if m.top > 10 || m.top < 10-m.logHeight() {
		t.Errorf("top = %d; first match (line 10) should be on screen", m.top)
	}
	m = keys(m, "n")
	if m.mi != 1 || m.top > 50 || m.top+m.logHeight() <= 50 {
		t.Errorf("after n: mi=%d top=%d; want line 50 on screen", m.mi, m.top)
	}
	m = keys(m, "N", "N")
	if m.mi != 3 {
		t.Errorf("N from first should wrap to last, mi=%d", m.mi)
	}
	m = keys(m, "n")
	if m.mi != 0 {
		t.Errorf("n from last should wrap to first, mi=%d", m.mi)
	}
	if v := m.View(); !strings.Contains(v, "match 1/4") {
		t.Errorf("title should show the match position:\n%s", v)
	}
}

func TestLogSearchNoMatchAndEscClears(t *testing.T) {
	m := keys(logModel(), "/", "z", "z", "z", "enter")
	if len(m.matches) != 0 || !strings.Contains(m.status, "no match") {
		t.Errorf("matches=%v status=%q", m.matches, m.status)
	}
	m = keys(m, "n") // must not panic with no matches
	m = keys(logModel(), "/", "e", "r", "r", "enter", "esc")
	if m.query != "" || m.matches != nil || m.view != viewLogs {
		t.Errorf("esc should clear the search and stay in the logs: query=%q view=%v", m.query, m.view)
	}
	if m = keys(m, "esc"); m.view != viewGraph {
		t.Errorf("second esc should leave the log view, got %v", m.view)
	}
}

func TestLogSearchSurvivesReload(t *testing.T) {
	m := keys(logModel(), "/", "e", "r", "r", "o", "r", "enter")
	m.logs = append(m.logs, "late error line")
	next, _ := m.Update(dataMsg{view: viewLogs, logs: m.logs})
	m = next.(model)
	if len(m.matches) != 5 {
		t.Errorf("matches after reload = %v; want 5", m.matches)
	}
}

func TestLogSearchRefiningQueryKeepsIndexInRange(t *testing.T) {
	m := keys(logModel(), "/", "l", "i", "n", "e", " ", "9", "enter") // lines 9, 90-99
	m = keys(m, "n", "n", "n", "n", "n")                              // mi = 5
	if m.mi != 5 || len(m.matches) != 11 {
		t.Fatalf("setup: mi=%d matches=%d", m.mi, len(m.matches))
	}
	m = keys(m, "/", "0") // refine to "line 90" while typing: one match left
	_ = m.View()          // must not index past the shrunken match list
	if m.mi >= len(m.matches) {
		t.Errorf("mi=%d out of range for %d matches", m.mi, len(m.matches))
	}
}
