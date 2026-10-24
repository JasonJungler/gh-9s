package ghcli

import (
	"reflect"
	"testing"
)

func TestRunActionArgs(t *testing.T) {
	gh := RepoRef{Owner: "acme", Name: "api"}
	ghe := RepoRef{Host: "ghe.corp", Owner: "acme", Name: "api"}
	cases := []struct {
		r    RepoRef
		a    RunAction
		want []string
	}{
		{gh, RunRerun, []string{"run", "rerun", "123", "-R", "acme/api"}},
		{gh, RunRerunFailed, []string{"run", "rerun", "123", "--failed", "-R", "acme/api"}},
		{gh, RunCancel, []string{"run", "cancel", "123", "-R", "acme/api"}},
		{gh, RunDelete, []string{"run", "delete", "123", "-R", "acme/api"}},
		{ghe, RunRerunFailed, []string{"run", "rerun", "123", "--failed", "-R", "ghe.corp/acme/api"}},
		{RepoRef{}, RunCancel, []string{"run", "cancel", "123"}},
	}
	for _, c := range cases {
		if got := RunActionArgs(c.r, "123", c.a); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s on %+v: %q; want %q", c.a, c.r, got, c.want)
		}
	}
	if RunActionArgs(gh, "123", RunAction("explode")) != nil {
		t.Error("an unknown action builds nothing")
	}
	if RunActionArgs(gh, "--web", RunCancel) != nil || RunActionArgs(gh, "", RunCancel) != nil {
		t.Error("a run id that is not digits builds nothing: it could be read as a flag")
	}
}
