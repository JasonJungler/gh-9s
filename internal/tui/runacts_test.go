package tui

import (
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func TestRunRule(t *testing.T) {
	type c struct {
		action             ghcli.RunAction
		status, conclusion string
		ok                 bool
		why                string
	}
	cases := []c{
		{ghcli.RunRerun, "completed", "success", true, ""},
		{ghcli.RunRerun, "completed", "failure", true, ""},
		{ghcli.RunRerun, "in_progress", "", false, "in progress"},
		{ghcli.RunRerun, "queued", "", false, "in progress"},
		{ghcli.RunRerunFailed, "completed", "failure", true, ""},
		{ghcli.RunRerunFailed, "completed", "timed_out", true, ""},
		{ghcli.RunRerunFailed, "completed", "cancelled", true, ""},
		{ghcli.RunRerunFailed, "completed", "success", false, "nothing failed"},
		{ghcli.RunRerunFailed, "IN_PROGRESS", "", false, "in progress"},
		{ghcli.RunCancel, "in_progress", "", true, ""},
		{ghcli.RunCancel, "queued", "", true, ""},
		{ghcli.RunCancel, "waiting", "", true, ""},
		{ghcli.RunCancel, "completed", "failure", false, "already finished"},
		{ghcli.RunDelete, "completed", "failure", true, ""},
		{ghcli.RunDelete, "completed", "success", true, ""},
		{ghcli.RunDelete, "in_progress", "", false, "cancel it first"},
	}
	for _, x := range cases {
		ok, why := runRule(x.action, x.status, x.conclusion)
		if ok != x.ok || (x.why == "") != (why == "") || !strings.Contains(why, x.why) {
			t.Errorf("%s %s/%s: ok=%v why=%q; want ok=%v why~%q", x.action, x.status, x.conclusion, ok, why, x.ok, x.why)
		}
	}
	if ok, _ := runRule(ghcli.RunAction("x"), "completed", "failure"); ok {
		t.Error("an unknown action never applies")
	}
}
