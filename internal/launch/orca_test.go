package launch

import (
	"errors"
	"strings"
	"testing"
)

// The outputs below are what Orca 1.4.238 printed (shortened to the fields the
// launcher reads; see the comments above the parsers in orca.go).

const orcaWorktreeID = "42a91ff8-9692-4cc1-965e-16e8ba8703b3::/Users/me/orca/workspaces/gh-9s/issue-42"

const orcaTerm1 = "term_8f0d1538-a4b6-43b8-9b55-9056fc57fb93"
const orcaTerm2 = "term_30a4f63f-acbd-45a4-97ac-27a646f7fb61"

const orcaOp = "1792921284877-3bf7854de9154a3687fdac0818e337a1"

const orcaCreateAgentOut = `{
  "id": "16fc57ec-8958-40dd-b5cd-a45d9ad05c32",
  "ok": true,
  "result": {
    "worktree": {
      "id": "` + orcaWorktreeID + `",
      "instanceId": "b213a44f-e77d-41ed-99ac-c986e4c040bc",
      "path": "/Users/me/orca/workspaces/gh-9s/issue-42",
      "branch": "refs/heads/MurabitoB/issue-42",
      "displayName": "issue-42",
      "linkedIssue": 42,
      "baseRef": "refs/remotes/origin/main"
    },
    "workspaceLineage": null,
    "launch": {
      "outcome": {"kind": "terminal", "handle": "` + orcaTerm1 + `", "paneKey": "6051bf8b:3515b7a7"},
      "worktreeId": "` + orcaWorktreeID + `",
      "receipt": {"mode": "terminal", "preferred": "terminal", "reason": "user_default"},
      "prompt": {"delivery": "submit", "outcome": "handed-to-terminal"},
      "operationId": "` + orcaOp + `"
    }
  },
  "_meta": {"runtimeId": "5c7d077f-2355-4f7a-b684-82f931b9c7f0"}
}
`

// orcaCreateBareOut is the reply of worktree create without --agent.
const orcaCreateBareOut = `{
  "id": "16fc57ec-8958-40dd-b5cd-a45d9ad05c32",
  "ok": true,
  "result": {
    "worktree": {
      "id": "` + orcaWorktreeID + `",
      "path": "/Users/me/orca/workspaces/gh-9s/issue-42",
      "branch": "refs/heads/MurabitoB/issue-42",
      "displayName": "issue-42"
    },
    "workspaceLineage": null
  }
}
`

const orcaAgentLaunchOut = `{
  "id": "f0e4dd26-99ac-4e4a-8aa4-7dd840a61921",
  "ok": true,
  "result": {
    "outcome": {"kind": "terminal", "handle": "` + orcaTerm2 + `", "paneKey": "4ea67ccc:c4393a9d"},
    "worktreeId": "` + orcaWorktreeID + `",
    "receipt": {"mode": "terminal", "preferred": "terminal", "reason": "user_default"},
    "placement": {"groupId": "66ce7f7c-72d8-4a8b-815c-f85d4408d242"},
    "operationId": "1792921346166-09a8e2bd55644f878afce7738e25a469"
  }
}
`

const orcaSendWarnOut = `{
  "id": "bd13274f-3c17-4787-ac6d-342f34e63d18",
  "ok": true,
  "result": {
    "send": {
      "handle": "` + orcaTerm2 + `",
      "accepted": true,
      "bytesWritten": 2011,
      "prompt": {"requestId": "7fbf659a-cfff-4fe3-b7e0-e7553deba62f", "stages": ["input_accepted"], "provider": "claude", "observation": "supported"}
    },
    "mutation": {"requestId": "7fbf659a-cfff-4fe3-b7e0-e7553deba62f", "replayed": false},
    "warnings": ["input was accepted but no turn start was observed, so the Enter may have been swallowed."]
  }
}
`

const orcaSendOkOut = `{"id":"x","ok":true,"result":{"send":{"handle":"` + orcaTerm2 + `","accepted":true,"bytesWritten":5},"mutation":{"replayed":false}}}`

// orcaFailOut is what orca printed on stdout (exit 1) for a failed command.
const orcaFailOut = `{
  "id": "20a1dcd8-918c-46b1-ba91-86f4dc071b26",
  "ok": false,
  "error": {"code": "repo_not_found", "message": "repo_not_found"},
  "_meta": {"runtimeId": "5c7d077f-2355-4f7a-b684-82f931b9c7f0"}
}
`

// orcaLostOut is the --json error of a lost launch reply, as built by Orca's
// CLI source (agent-launch-recovery.js).
const orcaLostOut = `{
  "id": "local",
  "ok": false,
  "error": {
    "code": "agent_session_operation_unknown",
    "message": "Orca could not confirm the launch outcome; the agent may already be running and the prompt may already be submitted.",
    "data": {"launchOutcome": "unknown", "retrySafe": true, "operationId": "` + orcaOp + `",
      "nextSteps": ["Re-issue the exact same command from the same directory against the same execution host with --retry-launch ` + orcaOp + `."]}
  },
  "_meta": {"runtimeId": null}
}
`

// orcaWaitOkOut is a satisfied `terminal wait` reply. It is hand-written from
// Orca's CLI source (result.wait.satisfied); only the timeout and stale-handle
// failures below were recorded from the real tool.
const orcaWaitOkOut = `{"id":"x","ok":true,"result":{"wait":{"satisfied":true,"status":"idle"}}}`

// orcaWaitTimeoutOut was recorded from `orca terminal wait --terminal <busy>
// --for tui-idle --timeout-ms 1500 --json` (exit 1).
const orcaWaitTimeoutOut = `{
  "id": "211876a2-efb6-46ab-9240-a424d4a7dabe",
  "ok": false,
  "error": {"code": "timeout", "message": "timeout"},
  "_meta": {"runtimeId": "5c7d077f-2355-4f7a-b684-82f931b9c7f0"}
}
`

// fallbackReply answers the four calls of the large-context fallback.
func fallbackReply(send string) func(c call) ([]byte, error) {
	return func(c call) ([]byte, error) {
		switch {
		case c.args[0] == "worktree":
			return []byte(orcaCreateBareOut), nil
		case c.args[0] == "agent":
			return []byte(orcaAgentLaunchOut), nil
		case c.args[0] == "terminal" && c.args[1] == "wait":
			return []byte(orcaWaitOkOut), nil
		}
		return []byte(send), nil
	}
}

func orcaReq() Request {
	claude, _ := LookupAgent("claude")
	return Request{
		Target: TargetOrca, Agent: claude, Context: "fix it\nplease",
		Dir: "/work", Base: "origin/main", Name: "issue-42", Issue: 42,
	}
}

func TestOrcaCreateArgs(t *testing.T) {
	r := orcaReq()
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	if _, err := (orcaBackend{run: rec.run}).launch(r); err != nil {
		t.Fatal(err)
	}
	want := []string{"worktree", "create", "--name", "issue-42", "--base-branch", "origin/main",
		"--issue", "42", "--agent", "claude", "--prompt", "fix it\nplease", "--json"}
	if len(rec.calls) != 1 || rec.calls[0].name != "orca" || !equalArgs(rec.calls[0].args, want) {
		t.Fatalf("calls = %+v, want one orca %q", rec.calls, want)
	}
	if rec.calls[0].dir != "/work" {
		t.Errorf("dir = %q, want /work (so orca infers the repo)", rec.calls[0].dir)
	}

	r.Model = "opus"
	rec = &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	if _, err := (orcaBackend{run: rec.run}).launch(r); err != nil {
		t.Fatal(err)
	}
	want = []string{"worktree", "create", "--name", "issue-42", "--base-branch", "origin/main",
		"--issue", "42", "--agent", "claude", "--model", "opus", "--prompt", "fix it\nplease", "--json"}
	if !equalArgs(rec.calls[0].args, want) {
		t.Errorf("args with a model = %q, want %q", rec.calls[0].args, want)
	}
}

func TestOrcaUsesOrcaAgentID(t *testing.T) {
	r := orcaReq()
	r.Agent = Agent{Label: "X", OrcaID: "codex-x"}
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	if _, err := (orcaBackend{run: rec.run}).launch(r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.calls[0].line(), "--agent codex-x ") {
		t.Errorf("call = %s", rec.calls[0].line())
	}
}

func TestOrcaReadsBranchFromResult(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	res, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "MurabitoB/issue-42" {
		t.Errorf("Branch = %q, want MurabitoB/issue-42 (from the result, refs/heads/ stripped)", res.Branch)
	}
	if res.Where != "issue-42" {
		t.Errorf("Where = %q, want the workspace name", res.Where)
	}
	if res.Note != "" {
		t.Errorf("Note = %q, want none", res.Note)
	}
}

func TestOrcaLargeContextFallsBack(t *testing.T) {
	r := orcaReq()
	r.Model = "opus"
	r.Context = strings.Repeat("字", orcaPromptMax/3+1) // 131073 bytes, more than orcaPromptMax
	if len(r.Context) <= orcaPromptMax {
		t.Fatalf("test context is %d bytes", len(r.Context))
	}
	rec := &recorder{reply: fallbackReply(orcaSendWarnOut)}
	res, err := (orcaBackend{run: rec.run}).launch(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 4 {
		t.Fatalf("calls = %d, want 4 (create, launch, wait, send): %+v", len(rec.calls), rec.calls)
	}
	create := []string{"worktree", "create", "--name", "issue-42", "--base-branch", "origin/main", "--issue", "42", "--json"}
	if !equalArgs(rec.calls[0].args, create) {
		t.Errorf("create = %q, want %q (no --agent, no --prompt)", rec.calls[0].args, create)
	}
	launch := []string{"agent", "launch", "--agent", "claude", "--worktree", "id:" + orcaWorktreeID, "--model", "opus", "--json"}
	if !equalArgs(rec.calls[1].args, launch) {
		t.Errorf("launch = %q, want %q", rec.calls[1].args, launch)
	}
	wait := []string{"terminal", "wait", "--terminal", orcaTerm2, "--for", "tui-idle", "--timeout-ms", "30000", "--json"}
	if !equalArgs(rec.calls[2].args, wait) {
		t.Errorf("wait = %q, want %q (before the send)", rec.calls[2].args, wait)
	}
	send := []string{"terminal", "send", "--terminal", orcaTerm2, "--text", r.Context, "--enter", "--json"}
	if !equalArgs(rec.calls[3].args, send) {
		t.Errorf("send args differ from the expected terminal send (text length %d)", len(rec.calls[3].args[4]))
	}
	for _, c := range rec.calls {
		if c.dir != "/work" {
			t.Errorf("%s ran in %q", c.line()[:20], c.dir)
		}
	}
	if res.Branch != "MurabitoB/issue-42" || res.Where != "issue-42" {
		t.Errorf("res = %+v", res)
	}
	if res.Note == "" {
		t.Error("orca's warning that it did not see the prompt start was dropped")
	}
}

func TestOrcaPromptBoundary(t *testing.T) {
	if orcaPromptMax != 131071 {
		t.Fatalf("orcaPromptMax = %d, want 131071", orcaPromptMax)
	}
	for _, tc := range []struct {
		size  int
		calls int
		flag  bool
	}{{orcaPromptMax, 1, true}, {orcaPromptMax + 1, 4, false}} {
		r := orcaReq()
		r.Context = strings.Repeat("a", tc.size)
		rec := &recorder{reply: func(c call) ([]byte, error) {
			if c.args[0] == "worktree" && c.args[2] != "" && strings.Contains(strings.Join(c.args, " "), "--prompt") {
				return []byte(orcaCreateAgentOut), nil
			}
			return fallbackReply(orcaSendOkOut)(c)
		}}
		if _, err := (orcaBackend{run: rec.run}).launch(r); err != nil {
			t.Fatal(err)
		}
		hasPrompt := strings.Contains(strings.Join(rec.calls[0].args, " "), "--prompt")
		if len(rec.calls) != tc.calls || hasPrompt != tc.flag {
			t.Errorf("%d bytes: %d calls, --prompt = %v; want %d calls, --prompt = %v", tc.size, len(rec.calls), hasPrompt, tc.calls, tc.flag)
		}
	}
}

func TestOrcaFallbackSendWithoutWarningHasNoNote(t *testing.T) {
	r := orcaReq()
	r.Context = strings.Repeat("a", orcaPromptMax+1)
	rec := &recorder{reply: fallbackReply(orcaSendOkOut)}
	res, err := (orcaBackend{run: rec.run}).launch(r)
	if err != nil || res.Note != "" {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestOrcaFallbackStepFailures(t *testing.T) {
	big := orcaReq()
	big.Context = strings.Repeat("a", orcaPromptMax+1)
	// upto answers well for the first n calls, then runs bad.
	upto := func(n int, bad func(c call) ([]byte, error)) func(c call) ([]byte, error) {
		i := 0
		return func(c call) ([]byte, error) {
			i++
			if i > n {
				return bad(c)
			}
			return fallbackReply(orcaSendOkOut)(c)
		}
	}
	fail := func(c call) ([]byte, error) { return []byte(orcaFailOut), errors.New("exit status 1") }
	cases := []struct {
		name  string
		reply func(c call) ([]byte, error)
		step  string
		calls int
		want  string
	}{
		{"create fails", upto(0, fail), StepCreate, 1, "repo_not_found"},
		{"agent launch fails", upto(1, fail), StepStart, 2, "was created"},
		{"agent started as a chat", upto(1, func(c call) ([]byte, error) {
			return []byte(`{"ok":true,"result":{"outcome":{"kind":"chat"}}}`), nil
		}), StepStart, 2, "chat"},
		{"wait times out", upto(2, func(c call) ([]byte, error) {
			return []byte(orcaWaitTimeoutOut), errors.New("exit status 1")
		}), StepReady, 3, "timeout"},
		{"wait not satisfied", upto(2, func(c call) ([]byte, error) {
			return []byte(`{"ok":true,"result":{"wait":{"satisfied":false,"status":"blocked"}}}`), errors.New("exit status 1")
		}), StepReady, 3, "did not become ready"},
		{"wait exits 0 but not satisfied", upto(2, func(c call) ([]byte, error) {
			return []byte(`{"ok":true,"result":{"wait":{"satisfied":false}}}`), nil
		}), StepReady, 3, "did not become ready"},
		{"send fails", upto(3, func(c call) ([]byte, error) { return nil, errors.New("terminal gone") }),
			StepPaste, 4, "context was not sent"},
		{"send not accepted", upto(3, func(c call) ([]byte, error) {
			return []byte(`{"ok":true,"result":{"send":{"accepted":false}}}`), nil
		}), StepPaste, 4, "did not accept"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: tc.reply}
			_, err := (orcaBackend{run: rec.run}).launch(big)
			var se *StepError
			if !errors.As(err, &se) || se.Step != tc.step {
				t.Fatalf("err = %v, want a %q step error", err, tc.step)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
			if len(rec.calls) != tc.calls {
				t.Errorf("calls = %d, want %d (nothing retried, nothing after the failure)", len(rec.calls), tc.calls)
			}
			if tc.step == StepReady {
				for _, c := range rec.calls {
					if len(c.args) > 1 && c.args[1] == "send" {
						t.Errorf("the context was sent although the agent never became ready: %s", c.line()[:40])
					}
				}
			}
		})
	}
}

// A lost reply from the fallback's agent launch must not offer a retry, since
// the fallback always refuses a RetryID.
func TestOrcaFallbackLostLaunchReplyIsNotRetryable(t *testing.T) {
	big := orcaReq()
	big.Context = strings.Repeat("a", orcaPromptMax+1)
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "worktree" {
			return []byte(orcaCreateBareOut), nil
		}
		return []byte(orcaLostOut), errors.New("exit status 1")
	}}
	_, err := (orcaBackend{run: rec.run}).launch(big)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart {
		t.Fatalf("err = %v, want a start step error", err)
	}
	var lost *LostReplyError
	if errors.As(err, &lost) {
		t.Errorf("a LostReplyError is reachable, so a caller would offer a retry that is always refused: %v", err)
	}
	text := err.Error()
	if strings.Contains(text, "--retry-launch") || strings.Contains(text, orcaOp) {
		t.Errorf("message offers a retry: %q", text)
	}
	for _, want := range []string{"issue-42", "/Users/me/orca/workspaces/gh-9s/issue-42", "not confirmed", "by hand"} {
		if !strings.Contains(text, want) {
			t.Errorf("message %q does not mention %q", text, want)
		}
	}
	if len(rec.calls) != 2 {
		t.Errorf("calls = %d, want 2", len(rec.calls))
	}
}

func TestOrcaFallbackRefusesRetryID(t *testing.T) {
	r := orcaReq()
	r.Context = strings.Repeat("a", orcaPromptMax+1)
	r.RetryID = orcaOp
	rec := &recorder{}
	_, err := (orcaBackend{run: rec.run}).launch(r)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate || len(rec.calls) != 0 {
		t.Errorf("err = %v, calls = %+v", err, rec.calls)
	}
}

func TestOrcaLostReplyOffersRetry(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaLostOut), errors.New("exit status 1") }}
	_, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate {
		t.Fatalf("err = %v, want a create step error", err)
	}
	if !strings.Contains(se.Err.Error(), "--retry-launch "+orcaOp) {
		t.Errorf("Err = %q, want it to mention --retry-launch %s", se.Err, orcaOp)
	}
	var lost *LostReplyError
	if !errors.As(err, &lost) || lost.OperationID != orcaOp {
		t.Errorf("err does not carry the operation id: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("calls = %d, want 1: nothing is retried silently", len(rec.calls))
	}
	first := rec.calls[0].args

	// The retry: the same command with --retry-launch <id> added.
	r := orcaReq()
	r.RetryID = orcaOp
	rec = &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	res, err := (orcaBackend{run: rec.run}).launch(r)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]string{}, first...), "--retry-launch", orcaOp)
	if len(rec.calls) != 1 || !equalArgs(rec.calls[0].args, want) {
		t.Errorf("retry args = %q, want %q", rec.calls[0].args, want)
	}
	if res.Branch != "MurabitoB/issue-42" {
		t.Errorf("res = %+v", res)
	}
}

func TestOrcaUnknownOutcomeWithoutAValidIDIsNotRetryable(t *testing.T) {
	for _, data := range []string{`{}`, `{"operationId":"abc"}`, `{"operationId":"--json"}`, `null`} {
		out := `{"ok":false,"error":{"code":"agent_session_operation_unknown","message":"unknown","data":` + data + `}}`
		var lost *LostReplyError
		err := orcaFailure([]byte(out), errors.New("exit status 1"))
		if errors.As(err, &lost) {
			t.Errorf("data %s: must not offer a retry: %v", data, err)
		}
	}
	// An id only in free text (no JSON on stdout) is not trusted either.
	err := orcaFailure(nil, errors.New("Orca could not confirm the launch outcome\nNext step: ... --retry-launch "+orcaOp))
	var lost *LostReplyError
	if errors.As(err, &lost) {
		t.Errorf("an id in free text must not make a lost reply: %v", err)
	}
}

func TestOrcaRejectsMalformedRetryID(t *testing.T) {
	for _, id := range []string{"abc", "--json", orcaOp + " x", "1792921284877-3BF7854DE9154A3687FDAC0818E337A1"} {
		r := orcaReq()
		r.RetryID = id
		rec := &recorder{}
		_, err := (orcaBackend{run: rec.run}).launch(r)
		var se *StepError
		if !errors.As(err, &se) || len(rec.calls) != 0 {
			t.Errorf("RetryID %q: err = %v, calls = %d", id, err, len(rec.calls))
		}
	}
}

func TestOrcaPlainFailureShowsOrcasMessage(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaFailOut), errors.New("exit status 1") }}
	_, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate || !strings.Contains(err.Error(), "repo_not_found") {
		t.Errorf("err = %v", err)
	}
	var lost *LostReplyError
	if errors.As(err, &lost) {
		t.Error("a plain failure is not a lost reply")
	}
}

func TestOrcaRejectsOptionLookingAndControlArguments(t *testing.T) {
	cases := []struct {
		name string
		mod  func(r *Request)
	}{
		{"name starts with dash", func(r *Request) { r.Name = "--evil" }},
		{"name with newline", func(r *Request) { r.Name = "a\nb" }},
		{"name with escape", func(r *Request) { r.Name = "a\x1bb" }},
		{"name with DEL", func(r *Request) { r.Name = "a\x7fb" }},
		{"empty name", func(r *Request) { r.Name = "" }},
		{"base starts with dash", func(r *Request) { r.Base = "-x" }},
		{"base with control char", func(r *Request) { r.Base = "origin/ma\tin" }},
		{"empty base", func(r *Request) { r.Base = "" }},
		{"model starts with dash", func(r *Request) { r.Model = "--effort" }},
		{"model with control char", func(r *Request) { r.Model = "op\x00us" }},
		{"agent without orca id", func(r *Request) { r.Agent.OrcaID = "" }},
		{"context starts with dash", func(r *Request) {}}, // the loop sets the context
	}
	for _, tc := range cases {
		for _, size := range []int{10, orcaPromptMax + 1} {
			t.Run(tc.name, func(t *testing.T) {
				r := orcaReq()
				r.Context = strings.Repeat("a", size)
				tc.mod(&r)
				if tc.name == "context starts with dash" {
					r.Context = "-" + strings.Repeat("a", size-1)
				}
				rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
				_, err := (orcaBackend{run: rec.run}).launch(r)
				var se *StepError
				if !errors.As(err, &se) || se.Step != StepCreate {
					t.Errorf("err = %v, want a create step error", err)
				}
				if len(rec.calls) != 0 {
					t.Errorf("orca was called: %+v", rec.calls[0].line())
				}
			})
		}
	}
}

func TestOrcaAcceptsNameWithSpacesAndUnicode(t *testing.T) {
	r := orcaReq()
	r.Name = "issue-42-修正 login"
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	if _, err := (orcaBackend{run: rec.run}).launch(r); err != nil {
		t.Fatal(err)
	}
	if rec.calls[0].args[3] != r.Name {
		t.Errorf("name arg = %q", rec.calls[0].args[3])
	}
}

func TestOrcaUnexpectedCreateOutput(t *testing.T) {
	cases := map[string]string{
		"empty":         ``,
		"text":          `created!`,
		"not ok":        `{"ok":false,"result":{}}`,
		"no worktree":   `{"ok":true,"result":{}}`,
		"odd id":        `{"ok":true,"result":{"worktree":{"id":"not-an-id","branch":"refs/heads/x"}}}`,
		"no branch":     `{"ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","branch":""}}}`,
		"branch spaces": `{"ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","branch":"refs/heads/a b"}}}`,
		"branch ctrl":   `{"ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","branch":"refs/heads/a\u001bb"}}}`,
		"result string": `{"ok":true,"result":"x"}`,
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(out), nil }}
			_, err := (orcaBackend{run: rec.run}).launch(orcaReq())
			var se *StepError
			if !errors.As(err, &se) || se.Step != StepCreate {
				t.Errorf("err = %v, want a create step error", err)
			}
			if len(rec.calls) != 1 {
				t.Errorf("calls = %d, want 1", len(rec.calls))
			}
		})
	}
}

func TestOrcaCreateWithoutAgentLaunchIsAStartError(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateBareOut), nil }}
	_, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart {
		t.Fatalf("err = %v, want a start step error", err)
	}
	if !strings.Contains(err.Error(), "issue-42") {
		t.Errorf("err = %v, want it to name the workspace that was created", err)
	}
}

func TestOrcaOldHostTerminalHandle(t *testing.T) {
	out := `{"ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","branch":"refs/heads/b","displayName":"n"},"agentTerminalHandle":"` + orcaTerm1 + `"}}`
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(out), nil }}
	res, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	if err != nil || res.Branch != "b" {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestOrcaWhereFallsBackToPath(t *testing.T) {
	out := `{"ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","path":"/p/w","branch":"refs/heads/b"},"agentTerminalHandle":"` + orcaTerm1 + `"}}`
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(out), nil }}
	res, err := (orcaBackend{run: rec.run}).launch(orcaReq())
	if err != nil || res.Where != "/p/w" {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

const orcaRepoShowOut = `{
  "id": "1aab5715-f3dd-4d35-a0e7-6d445736c3e8",
  "ok": true,
  "result": {"repo": {"id": "42a91ff8-9692-4cc1-965e-16e8ba8703b3", "path": "/Users/me/orca/projects/gh-9s",
    "displayName": "gh-9s", "kind": "git", "externalWorktreeVisibility": "hide"}}
}
`

func repoShowWithBase(base string) string {
	return strings.Replace(orcaRepoShowOut, `"kind": "git",`, `"kind": "git", "worktreeBaseRef": "`+base+`",`, 1)
}

// orcaWorktreeShowOut is a minimal hand-written reply, not recorded from Orca
// (the real one has many more fields; only worktree.repoId is read).
const orcaWorktreeShowOut = `{"id":"x","ok":true,"result":{"worktree":{"id":"` + orcaWorktreeID + `","repoId":"42a91ff8-9692-4cc1-965e-16e8ba8703b3"}}}`

func TestOrcaDefaultBase(t *testing.T) {
	t.Run("field present", func(t *testing.T) {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(repoShowWithBase("origin/develop")), nil }}
		base, ok := OrcaDefaultBase(rec.run, "/work")
		if base != "origin/develop" || !ok {
			t.Errorf("= %q, %v", base, ok)
		}
		want := []string{"repo", "show", "--repo", "path:/work", "--json"}
		if len(rec.calls) != 1 || rec.calls[0].name != "orca" || !equalArgs(rec.calls[0].args, want) {
			t.Errorf("calls = %+v, want orca %q", rec.calls, want)
		}
	})
	t.Run("field absent", func(t *testing.T) {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaRepoShowOut), nil }}
		if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
			t.Errorf("= %q, %v", base, ok)
		}
		if len(rec.calls) != 1 {
			t.Errorf("calls = %d: a repo that was found but has no base must not trigger the worktree lookup", len(rec.calls))
		}
	})
	t.Run("field blank", func(t *testing.T) {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(repoShowWithBase("  ")), nil }}
		if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
			t.Errorf("= %q, %v", base, ok)
		}
	})
	t.Run("unusable value", func(t *testing.T) {
		for _, v := range []string{"-x", `a\u001bb`} {
			rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(repoShowWithBase(v)), nil }}
			if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
				t.Errorf("%q = %q, %v", v, base, ok)
			}
		}
	})
	t.Run("errors", func(t *testing.T) {
		rec := &recorder{reply: func(c call) ([]byte, error) { return nil, errors.New("orca: not found") }}
		if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
			t.Errorf("= %q, %v", base, ok)
		}
	})
	t.Run("garbage", func(t *testing.T) {
		for _, out := range []string{``, `nope`, `{"ok":false}`, `{"ok":true,"result":{}}`, `{"ok":true,"result":[]}`} {
			rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(out), nil }}
			if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
				t.Errorf("%q = %q, %v", out, base, ok)
			}
		}
	})
}

func TestOrcaDefaultBaseInALinkedWorktree(t *testing.T) {
	// repo show --repo path:<linked worktree> fails with repo_not_found, so the
	// repo id is looked up from the worktree.
	rec := &recorder{reply: func(c call) ([]byte, error) {
		switch {
		case c.args[0] == "repo" && c.args[3] == "path:/work":
			return []byte(orcaFailOut), errors.New("exit status 1")
		case c.args[0] == "worktree":
			return []byte(orcaWorktreeShowOut), nil
		}
		return []byte(repoShowWithBase("origin/main")), nil
	}}
	base, ok := OrcaDefaultBase(rec.run, "/work")
	if base != "origin/main" || !ok {
		t.Errorf("= %q, %v", base, ok)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("calls = %+v", rec.calls)
	}
	if want := []string{"worktree", "show", "--worktree", "path:/work", "--json"}; !equalArgs(rec.calls[1].args, want) {
		t.Errorf("call 2 = %q, want %q", rec.calls[1].args, want)
	}
	if want := []string{"repo", "show", "--repo", "id:42a91ff8-9692-4cc1-965e-16e8ba8703b3", "--json"}; !equalArgs(rec.calls[2].args, want) {
		t.Errorf("call 3 = %q, want %q", rec.calls[2].args, want)
	}
}

func TestOrcaDefaultBaseRejectsOddRepoID(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "worktree" {
			return []byte(`{"ok":true,"result":{"worktree":{"repoId":"--evil"}}}`), nil
		}
		return nil, errors.New("exit status 1")
	}}
	if base, ok := OrcaDefaultBase(rec.run, "/work"); base != "" || ok {
		t.Errorf("= %q, %v", base, ok)
	}
	if len(rec.calls) != 2 {
		t.Errorf("calls = %d, an odd repo id must not be passed on", len(rec.calls))
	}
}

func TestOrcaOperationIDWithAnotherCodeIsNotALostReply(t *testing.T) {
	out := `{"ok":false,"error":{"code":"repo_not_found","message":"repo_not_found","data":{"operationId":"` + orcaOp + `"}}}`
	err := orcaFailure([]byte(out), errors.New("exit status 1"))
	var lost *LostReplyError
	if errors.As(err, &lost) {
		t.Errorf("only agent_session_operation_unknown is a lost reply: %v", err)
	}
	if !strings.Contains(err.Error(), "repo_not_found") {
		t.Errorf("err = %v", err)
	}
}

// Paste-only (launch_submit off) always takes the long path, whatever the size: create without an agent,
// launch the agent, wait for it to be idle, then send the text WITHOUT --enter so it stays in the input.
//
// Not verified live: the no-Enter send has never run against a real Orca (only unit-tested through the
// fake RunFunc here), and it is unknown whether `orca terminal send --text` delivers multi-line text as
// a bracketed paste. If it does not, a newline in the text may submit it.
func TestOrcaPasteOnlyUsesLongPathWithoutEnter(t *testing.T) {
	r := orcaReq()
	r.Model = "opus"
	r.PasteOnly = true
	if len(r.Context) > orcaPromptMax {
		t.Fatal("the context must be small: paste-only takes the long path at any size")
	}
	rec := &recorder{reply: fallbackReply(orcaSendOkOut)}
	res, err := (orcaBackend{run: rec.run}).launch(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 4 {
		t.Fatalf("calls = %d, want 4 (create, launch, wait, send): %+v", len(rec.calls), rec.calls)
	}
	if !equalArgs(rec.calls[0].args, []string{"worktree", "create", "--name", "issue-42", "--base-branch", "origin/main", "--issue", "42", "--json"}) {
		t.Errorf("create = %q (no --agent, no --prompt)", rec.calls[0].args)
	}
	if !equalArgs(rec.calls[1].args, []string{"agent", "launch", "--agent", "claude", "--worktree", "id:" + orcaWorktreeID, "--model", "opus", "--json"}) {
		t.Errorf("launch = %q", rec.calls[1].args)
	}
	if !equalArgs(rec.calls[2].args, []string{"terminal", "wait", "--terminal", orcaTerm2, "--for", "tui-idle", "--timeout-ms", "30000", "--json"}) {
		t.Errorf("wait = %q", rec.calls[2].args)
	}
	send := []string{"terminal", "send", "--terminal", orcaTerm2, "--text", r.Context, "--json"}
	if !equalArgs(rec.calls[3].args, send) {
		t.Errorf("send = %q, want %q", rec.calls[3].args, send)
	}
	for _, c := range rec.calls {
		for _, a := range c.args {
			if a == "--enter" || a == "--prompt" {
				t.Errorf("%q passes %s", c.line(), a)
			}
		}
	}
	if res.Branch != "MurabitoB/issue-42" || res.Where != "issue-42" || res.Note != "" {
		t.Errorf("res = %+v", res)
	}
}

// Without Enter, Orca's warning that no turn start was seen is expected, so it is not passed on as the
// usual "could not confirm the agent started" note.
func TestOrcaPasteOnlyDoesNotReportMissingTurnStart(t *testing.T) {
	r := orcaReq()
	r.PasteOnly = true
	rec := &recorder{reply: fallbackReply(orcaSendWarnOut)}
	res, err := (orcaBackend{run: rec.run}).launch(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Note, "started") {
		t.Errorf("Note = %q, it claims the agent should have started", res.Note)
	}
	if res.Note == "" {
		t.Error("orca's warning was dropped silently")
	}
}

func TestOrcaPasteOnlyStepFailures(t *testing.T) {
	r := orcaReq()
	r.PasteOnly = true
	upto := func(n int, bad func(c call) ([]byte, error)) func(c call) ([]byte, error) {
		i := 0
		return func(c call) ([]byte, error) {
			i++
			if i > n {
				return bad(c)
			}
			return fallbackReply(orcaSendOkOut)(c)
		}
	}
	fail := func(c call) ([]byte, error) { return []byte(orcaFailOut), errors.New("exit status 1") }
	cases := []struct {
		name  string
		reply func(c call) ([]byte, error)
		step  string
		calls int
	}{
		{"create fails", upto(0, fail), StepCreate, 1},
		{"agent launch fails", upto(1, fail), StepStart, 2},
		{"wait times out", upto(2, func(c call) ([]byte, error) {
			return []byte(orcaWaitTimeoutOut), errors.New("exit status 1")
		}), StepReady, 3},
		{"send fails", upto(3, func(c call) ([]byte, error) { return nil, errors.New("terminal gone") }), StepPaste, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: tc.reply}
			_, err := (orcaBackend{run: rec.run}).launch(r)
			var se *StepError
			if !errors.As(err, &se) || se.Step != tc.step {
				t.Fatalf("err = %v, want a %q step error", err, tc.step)
			}
			if len(rec.calls) != tc.calls {
				t.Errorf("calls = %d, want %d", len(rec.calls), tc.calls)
			}
		})
	}
}

// A lost reply from the agent launch is a plain StepError, as in the large-context fallback.
func TestOrcaPasteOnlyLostLaunchReplyIsNotRetryable(t *testing.T) {
	r := orcaReq()
	r.PasteOnly = true
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "worktree" {
			return []byte(orcaCreateBareOut), nil
		}
		return []byte(orcaLostOut), errors.New("exit status 1")
	}}
	_, err := (orcaBackend{run: rec.run}).launch(r)
	var se *StepError
	var lost *LostReplyError
	if !errors.As(err, &se) || se.Step != StepStart || errors.As(err, &lost) || strings.Contains(err.Error(), "--retry-launch") {
		t.Fatalf("err = %v, want a plain start step error with no retry", err)
	}
}

func TestOrcaPasteOnlyRefusesRetryID(t *testing.T) {
	r := orcaReq()
	r.PasteOnly = true
	r.RetryID = orcaOp
	rec := &recorder{}
	_, err := (orcaBackend{run: rec.run}).launch(r)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate || len(rec.calls) != 0 {
		t.Fatalf("err = %v, calls = %+v", err, rec.calls)
	}
	if strings.Contains(err.Error(), "too long") {
		t.Errorf("err = %q blames the size, but the context is small", err)
	}
}

// Submitting (PasteOnly false) is unchanged: a small context is one call with --prompt.
func TestOrcaSubmitKeepsTheOneCallPath(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	if _, err := (orcaBackend{run: rec.run}).launch(orcaReq()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || !strings.Contains(strings.Join(rec.calls[0].args, " "), "--prompt") {
		t.Errorf("calls = %+v", rec.calls)
	}
}
