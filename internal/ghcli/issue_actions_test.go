package ghcli

import (
	"reflect"
	"testing"
)

func TestCloseAndReopenArgs(t *testing.T) {
	if got, want := CloseIssueArgs(acmeAPI, "14812"), []string{"issue", "close", "14812", "-R", "acme/api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("close = %q; want %q", got, want)
	}
	if got, want := ReopenIssueArgs(RepoRef{}, "7"), []string{"issue", "reopen", "7"}; !reflect.DeepEqual(got, want) {
		t.Errorf("reopen = %q; want %q", got, want)
	}
}

func TestEditIssueArgs(t *testing.T) {
	got := EditIssueArgs(acmeAPI, "14812",
		[]string{"needs triage 🚧", "bug"}, []string{"wontfix"}, []string{"erin"}, []string{"frank"})
	want := []string{"issue", "edit", "14812",
		"--add-label=needs triage 🚧", "--add-label=bug", "--remove-label=wontfix",
		"--add-assignee=erin", "--remove-assignee=frank", "-R", "acme/api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EditIssueArgs =\n %q\nwant\n %q", got, want)
	}
}

func TestEditIssueArgsEmptyListsAddNothing(t *testing.T) {
	got := EditIssueArgs(acmeAPI, "1", nil, nil, nil, nil)
	if len(got) != 5 { // issue edit 1 -R acme/api
		t.Errorf("args = %q; empty change lists must add no flags", got)
	}
}

func TestEditIssueArgsKeepHostileNamesInOneArgument(t *testing.T) {
	for _, name := range []string{"--web", "-x", "a b; rm -rf ~", "$(id)", "line\nbreak"} {
		got := EditIssueArgs(acmeAPI, "1", []string{name}, nil, nil, nil)
		found := false
		for _, a := range got {
			if a == "--add-label="+name {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was not kept as one --add-label= argument: %q", name, got)
		}
	}
}

func TestParseLabelNamesAndLogins(t *testing.T) {
	labels, err := ParseLabelNames([]byte(`[{"name":"bug"},{"name":"needs triage 🚧"},{"name":""}]`))
	if err != nil || !reflect.DeepEqual(labels, []string{"bug", "needs triage 🚧"}) {
		t.Errorf("labels = %q err %v; want the two names, the empty one dropped", labels, err)
	}
	logins, err := ParseLogins([]byte(`[{"login":"erin"},{"login":"frank"}]`))
	if err != nil || !reflect.DeepEqual(logins, []string{"erin", "frank"}) {
		t.Errorf("logins = %q err %v", logins, err)
	}
	if got, err := ParseLabelNames([]byte(`[]`)); err != nil || len(got) != 0 {
		t.Errorf("empty list: %v %v", got, err)
	}
	if _, err := ParseLogins([]byte("garbage")); err == nil {
		t.Error("garbage should be an error")
	}
}
