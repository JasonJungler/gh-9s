package ghcli

import (
	"encoding/json"
	"fmt"
)

// MergeMethod is how a PR is merged.
type MergeMethod string

const (
	MergeCommit MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

// MergeLevel says whether a PR can be merged.
type MergeLevel int

const (
	MergeOK      MergeLevel = iota // GitHub reports it clean
	MergeWarn                      // GitHub would accept it, but something is not green (not required)
	MergeBlocked                   // GitHub would refuse it
)

// MergeVerdict is a level plus the reasons a person should read.
type MergeVerdict struct {
	Level   MergeLevel
	Reasons []string
}

// EvaluateMerge turns GitHub's own verdict (state, mergeable, mergeStateStatus, reviewDecision)
// into a MergeVerdict. It adds no rules of its own: a PR is blocked exactly when GitHub says so,
// and anything it does not recognise is blocked rather than guessed to be fine.
func EvaluateMerge(d PRDetail) MergeVerdict {
	block := func(reasons ...string) MergeVerdict { return MergeVerdict{MergeBlocked, reasons} }
	switch d.State {
	case "MERGED":
		return block("it is already merged")
	case "CLOSED":
		return block("the PR is closed")
	}
	var reasons []string
	if d.IsDraft {
		reasons = append(reasons, "it is a draft")
	}
	if d.Mergeable == "CONFLICTING" || d.MergeState == "DIRTY" {
		reasons = append(reasons, "it has merge conflicts")
	}
	if len(reasons) > 0 {
		return block(reasons...)
	}
	switch d.MergeState {
	case "CLEAN", "HAS_HOOKS":
		return MergeVerdict{Level: MergeOK}
	case "UNSTABLE":
		return MergeVerdict{MergeWarn, []string{fmt.Sprintf(
			"non-required checks are failing or still running (✗%d ●%d)", d.ChecksFailed, d.ChecksPending)}}
	case "BEHIND":
		return block("the branch is behind its base; update it first")
	case "BLOCKED":
		switch d.ReviewDecision {
		case "REVIEW_REQUIRED":
			reasons = append(reasons, "a required review is missing")
		case "CHANGES_REQUESTED":
			reasons = append(reasons, "changes were requested")
		}
		if d.ChecksFailed > 0 {
			reasons = append(reasons, fmt.Sprintf("%d check(s) failing", d.ChecksFailed))
		}
		if d.ChecksPending > 0 {
			reasons = append(reasons, fmt.Sprintf("%d check(s) still running", d.ChecksPending))
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "branch protection requires something that is not done yet")
		}
		return block(reasons...)
	case "UNKNOWN", "":
		return block("GitHub is still working out the merge state; refresh in a moment")
	}
	return block("unknown merge state " + d.MergeState)
}

// ParseAllowedMethods reads the repo settings JSON. Settings the caller cannot see are absent
// and count as not allowed.
func ParseAllowedMethods(b []byte) ([]MergeMethod, error) {
	var s struct {
		Merge  bool `json:"allow_merge_commit"`
		Squash bool `json:"allow_squash_merge"`
		Rebase bool `json:"allow_rebase_merge"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	var out []MergeMethod
	if s.Merge {
		out = append(out, MergeCommit)
	}
	if s.Squash {
		out = append(out, MergeSquash)
	}
	if s.Rebase {
		out = append(out, MergeRebase)
	}
	return out, nil
}

// AllowedMergeMethods asks which merge methods the repo allows.
func AllowedMergeMethods(r RepoRef) ([]MergeMethod, error) {
	endpoint, flags := r.APIEndpoint("")
	out, err := Run(append(append([]string{"api"}, flags...), endpoint)...)
	if err != nil {
		return nil, err
	}
	return ParseAllowedMethods(out)
}

// MergeArgs is the `gh pr merge` command. It never passes --admin or --auto. headSHA, when
// given, makes GitHub refuse the merge if the PR has moved on since the user looked at it. An
// unknown method yields nil so nothing unintended can be built from a bad value.
func MergeArgs(r RepoRef, number string, m MergeMethod, deleteBranch bool, headSHA string) []string {
	var flag string
	switch m {
	case MergeCommit:
		flag = "--merge"
	case MergeSquash:
		flag = "--squash"
	case MergeRebase:
		flag = "--rebase"
	default:
		return nil
	}
	a := []string{"pr", "merge", number, flag}
	if deleteBranch {
		a = append(a, "--delete-branch")
	}
	if headSHA != "" {
		a = append(a, "--match-head-commit="+headSHA)
	}
	return Scoped(r, a...)
}

// MergePR merges the PR.
func MergePR(r RepoRef, number string, m MergeMethod, deleteBranch bool, headSHA string) error {
	args := MergeArgs(r, number, m, deleteBranch, headSHA)
	if args == nil {
		return fmt.Errorf("unknown merge method %q", m)
	}
	_, err := Run(args...)
	return err
}
