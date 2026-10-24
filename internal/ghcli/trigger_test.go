package ghcli

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func lister(batches ...[]RunInfo) (RunLister, *int) {
	calls := 0
	return func(RepoRef, string, string) ([]RunInfo, error) {
		i := min(calls, len(batches)-1)
		calls++
		return batches[i], nil
	}, &calls
}

func find(l RunLister, sleeps *[]time.Duration) (int64, error) {
	return FindTriggeredRun(l, RepoRef{}, "deploy.yml", "main", "me", t0,
		30*time.Second, 2*time.Second, func(d time.Duration) { *sleeps = append(*sleeps, d) })
}

func TestFindsNewestMatching(t *testing.T) {
	l, _ := lister([]RunInfo{
		{ID: 5, CreatedAt: t0.Add(4 * time.Second), Actor: "other", Event: "workflow_dispatch"},
		{ID: 4, CreatedAt: t0.Add(3 * time.Second), Actor: "me", Event: "push"},
		{ID: 3, CreatedAt: t0.Add(2 * time.Second), Actor: "me", Event: "workflow_dispatch"},
		{ID: 2, CreatedAt: t0.Add(1 * time.Second), Actor: "me", Event: "workflow_dispatch"},
		{ID: 1, CreatedAt: t0.Add(-time.Minute), Actor: "me", Event: "workflow_dispatch"},
	})
	var sleeps []time.Duration
	id, err := find(l, &sleeps)
	if err != nil || id != 3 {
		t.Errorf("id=%d err=%v; want 3", id, err)
	}
	if len(sleeps) != 0 {
		t.Errorf("should not sleep when found immediately: %v", sleeps)
	}
}

func TestPollsUntilAppears(t *testing.T) {
	l, calls := lister(nil, nil, []RunInfo{{ID: 9, CreatedAt: t0.Add(time.Second), Actor: "me", Event: "workflow_dispatch"}})
	var sleeps []time.Duration
	id, err := find(l, &sleeps)
	if err != nil || id != 9 || *calls != 3 || len(sleeps) != 2 || sleeps[0] != 2*time.Second {
		t.Errorf("id=%d err=%v calls=%d sleeps=%v", id, err, *calls, sleeps)
	}
}

func TestTimesOut(t *testing.T) {
	l, _ := lister(nil)
	var sleeps []time.Duration
	_, err := find(l, &sleeps)
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v; want ErrRunNotFound", err)
	}
	var total time.Duration
	for _, d := range sleeps {
		total += d
	}
	if total < 30*time.Second {
		t.Errorf("gave up after %v of waiting; want at least the 30s timeout", total)
	}
}

func TestListErrorSurfacesAtTimeout(t *testing.T) {
	boom := errors.New("api down")
	l := func(RepoRef, string, string) ([]RunInfo, error) { return nil, boom }
	var sleeps []time.Duration
	_, err := find(l, &sleeps)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v; want it to wrap the listing error", err)
	}
}
