package ghcli

import (
	"reflect"
	"strings"
	"testing"
)

func pr(state, mergeable, mergeState string) PRDetail {
	return PRDetail{Number: 1, State: state, Mergeable: mergeable, MergeState: mergeState}
}

func TestEvaluateMerge(t *testing.T) {
	draft := pr("OPEN", "MERGEABLE", "CLEAN")
	draft.IsDraft = true
	needsReview := pr("OPEN", "MERGEABLE", "BLOCKED")
	needsReview.ReviewDecision = "REVIEW_REQUIRED"
	changes := pr("OPEN", "MERGEABLE", "BLOCKED")
	changes.ReviewDecision = "CHANGES_REQUESTED"
	failing := pr("OPEN", "MERGEABLE", "BLOCKED")
	failing.ChecksFailed = 2
	pending := pr("OPEN", "MERGEABLE", "BLOCKED")
	pending.ChecksPending = 1
	unstable := pr("OPEN", "MERGEABLE", "UNSTABLE")
	unstable.ChecksFailed = 1

	cases := []struct {
		name   string
		d      PRDetail
		level  MergeLevel
		reason string // a word the explanation must contain ("" = none expected)
	}{
		{"clean", pr("OPEN", "MERGEABLE", "CLEAN"), MergeOK, ""},
		{"has hooks", pr("OPEN", "MERGEABLE", "HAS_HOOKS"), MergeOK, ""},
		{"unstable (non-required checks)", unstable, MergeWarn, "check"},
		{"blocked, review required", needsReview, MergeBlocked, "review"},
		{"blocked, changes requested", changes, MergeBlocked, "changes"},
		{"blocked, failing checks", failing, MergeBlocked, "check"},
		{"blocked, pending checks", pending, MergeBlocked, "check"},
		{"behind", pr("OPEN", "MERGEABLE", "BEHIND"), MergeBlocked, "behind"},
		{"dirty", pr("OPEN", "CONFLICTING", "DIRTY"), MergeBlocked, "conflict"},
		{"conflicting but clean state", pr("OPEN", "CONFLICTING", "CLEAN"), MergeBlocked, "conflict"},
		{"draft", draft, MergeBlocked, "draft"},
		{"merged", pr("MERGED", "UNKNOWN", "UNKNOWN"), MergeBlocked, "merged"},
		{"closed", pr("CLOSED", "MERGEABLE", "CLEAN"), MergeBlocked, "closed"},
		{"unknown (still computing)", pr("OPEN", "UNKNOWN", "UNKNOWN"), MergeBlocked, "refresh"},
		{"state missing", pr("OPEN", "", ""), MergeBlocked, "refresh"},
		{"a value we have never seen", pr("OPEN", "MERGEABLE", "SOMETHING_NEW"), MergeBlocked, "SOMETHING_NEW"},
	}
	for _, c := range cases {
		v := EvaluateMerge(c.d)
		if v.Level != c.level {
			t.Errorf("%s: level = %v; want %v (%v)", c.name, v.Level, c.level, v.Reasons)
			continue
		}
		joined := strings.ToLower(strings.Join(v.Reasons, " "))
		if c.reason == "" && len(v.Reasons) != 0 {
			t.Errorf("%s: an OK merge needs no reasons, got %v", c.name, v.Reasons)
		}
		if c.reason != "" && !strings.Contains(joined, strings.ToLower(c.reason)) {
			t.Errorf("%s: reasons %v should mention %q", c.name, v.Reasons, c.reason)
		}
	}
}

func TestMergeArgs(t *testing.T) {
	cases := []struct {
		r      RepoRef
		method MergeMethod
		del    bool
		want   []string
	}{
		{acmeAPI, MergeSquash, false, []string{"pr", "merge", "482", "--squash", "-R", "acme/api"}},
		{acmeAPI, MergeCommit, true, []string{"pr", "merge", "482", "--merge", "--delete-branch", "-R", "acme/api"}},
		{RepoRef{}, MergeRebase, false, []string{"pr", "merge", "482", "--rebase"}},
	}
	for _, c := range cases {
		got := MergeArgs(c.r, "482", c.method, c.del)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MergeArgs = %q; want %q", got, c.want)
		}
		for _, a := range got {
			if a == "--admin" || a == "--auto" {
				t.Errorf("release 2 never passes %s: %q", a, got)
			}
		}
	}
}

func TestParseAllowedMethods(t *testing.T) {
	cases := []struct {
		json string
		want []MergeMethod
	}{
		{`{"allow_merge_commit":true,"allow_squash_merge":true,"allow_rebase_merge":true}`, []MergeMethod{MergeCommit, MergeSquash, MergeRebase}},
		{`{"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":false}`, []MergeMethod{MergeSquash}},
		{`{"allow_merge_commit":false,"allow_squash_merge":false,"allow_rebase_merge":false}`, nil},
		{`{"name":"api"}`, nil}, // settings not visible: nothing is claimed to be allowed
	}
	for _, c := range cases {
		got, err := ParseAllowedMethods([]byte(c.json))
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseAllowedMethods(%s) = %v, %v; want %v", c.json, got, err, c.want)
		}
	}
	if _, err := ParseAllowedMethods([]byte("garbage")); err == nil {
		t.Error("garbage should be an error")
	}
}
