package ui

import "testing"

func TestRunState(t *testing.T) {
	cases := []struct {
		status, conclusion string
		label              string
		st                 State
	}{
		{"completed", "success", "✓ success", StOK},
		{"completed", "failure", "✗ failure", StFail},
		{"completed", "timed_out", "✗ timed_out", StFail},
		{"completed", "cancelled", "○ cancelled", StOther},
		{"in_progress", "", "● in_progress", StRun},
		{"queued", "", "● queued", StRun},
	}
	for _, c := range cases {
		label, st := RunState(c.status, c.conclusion)
		if label != c.label || st != c.st {
			t.Errorf("RunState(%q,%q) = %q,%v; want %q,%v", c.status, c.conclusion, label, st, c.label, c.st)
		}
	}
}

func TestDuration(t *testing.T) {
	cases := map[[2]string]string{
		{"2026-01-01T00:00:00Z", "2026-01-01T00:00:07Z"}: "7s",
		{"2026-01-01T00:00:00Z", "2026-01-01T00:02:05Z"}: "2m05s",
		{"", ""}: "-",
	}
	for in, want := range cases {
		if got := Duration(in[0], in[1]); got != want {
			t.Errorf("Duration(%q,%q) = %q; want %q", in[0], in[1], got, want)
		}
	}
}

func TestDurationEndBeforeStartIsZeroNotNow(t *testing.T) {
	// seen on skipped jobs: completedAt is a second before startedAt
	if got := Duration("2026-10-24T04:18:26Z", "2026-10-24T04:18:25Z"); got != "0s" {
		t.Errorf("Duration = %q; want 0s", got)
	}
	if got := Duration("", ""); got != "-" {
		t.Errorf("Duration of nothing = %q; want -", got)
	}
}
