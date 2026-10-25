package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// orcaPromptMax is the largest Context, in bytes, sent through `orca worktree
// create --prompt`. Spiked with Orca 1.4.238 and Claude Code: a 150 000 byte
// multi-line prompt (CJK, emoji, quotes, backslashes) was accepted by the CLI
// and reached the agent byte-for-byte (the session transcript held it
// unchanged), so the limit is the 128 KiB cap, not something Orca imposed.
// Longer text goes through the worktree create, agent launch, terminal send
// fallback.
const orcaPromptMax = 131072

// orcaWorktreeIDPattern is the shape of an Orca worktree id: the repo's uuid,
// "::", then the worktree's absolute path.
var orcaWorktreeIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}::/.+$`)

// orcaRepoIDPattern is the shape of an Orca repo id (a uuid).
var orcaRepoIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// orcaTerminalPattern is the shape of a terminal handle: "term_" and a uuid.
var orcaTerminalPattern = regexp.MustCompile(`^term_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// orcaOperationPattern is the shape of a launch operation id: 13 digits of
// milliseconds, "-", 32 hex digits. Orca's own CLI refuses --retry-launch
// values that do not match.
var orcaOperationPattern = regexp.MustCompile(`[0-9]{13}-[0-9a-f]{32}`)

var orcaOperationExact = regexp.MustCompile(`^[0-9]{13}-[0-9a-f]{32}$`)

// LostReplyError means Orca could not confirm whether a launch happened. The
// agent may be running already, so the only safe move is to ask Orca again with
// the same operation id (Request.RetryID), never to launch afresh.
type LostReplyError struct {
	OperationID string
	Cause       string
}

// Error describes the failure and names the retry flag.
func (e *LostReplyError) Error() string {
	return fmt.Sprintf("%s; the launch may already have happened, retry with --retry-launch %s", e.Cause, e.OperationID)
}

type orcaBackend struct{ run RunFunc }

// rejectArg reports why s cannot go to orca as the value of an option, or "".
// Values that start with "-" could be read as options, and control characters
// have no place in a name, ref or model id.
func rejectArg(s string) string {
	if strings.HasPrefix(s, "-") {
		return "starts with \"-\""
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return "contains a control character"
		}
	}
	return ""
}

// check validates everything that goes into orca's argv before any call.
func (o orcaBackend) check(r Request) error {
	for _, f := range []struct{ what, v string }{{"workspace name", r.Name}, {"base", r.Base}} {
		if f.v == "" {
			return fmt.Errorf("%s is empty", f.what)
		}
		if why := rejectArg(f.v); why != "" {
			return fmt.Errorf("%s %q %s", f.what, f.v, why)
		}
	}
	if why := rejectArg(r.Model); why != "" {
		return fmt.Errorf("model %q %s", r.Model, why)
	}
	if r.Agent.OrcaID == "" {
		return fmt.Errorf("%s has no orca agent id", r.Agent.Label)
	}
	if strings.HasPrefix(r.Context, "-") {
		return errors.New("the context starts with \"-\", which orca could read as an option")
	}
	return nil
}

// launch creates the workspace and starts the agent in it, then reports where
// it went. Whatever was created before a failure is left in place.
func (o orcaBackend) launch(r Request) (Result, error) {
	if err := o.check(r); err != nil {
		return Result{}, &StepError{StepCreate, err}
	}
	if len(r.Context) > orcaPromptMax {
		return o.launchLarge(r)
	}
	return o.launchOneCall(r)
}

func orcaCreateArgs(r Request) []string {
	args := []string{"worktree", "create", "--name", r.Name, "--base-branch", r.Base}
	if r.Issue > 0 {
		args = append(args, "--issue", fmt.Sprint(r.Issue))
	}
	return args
}

// launchOneCall is the normal path: one `worktree create` that also starts
// the agent with the context as its prompt.
func (o orcaBackend) launchOneCall(r Request) (Result, error) {
	args := append(orcaCreateArgs(r), "--agent", r.Agent.OrcaID)
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, "--prompt", r.Context, "--json")
	if r.RetryID != "" {
		if !orcaOperationExact.MatchString(r.RetryID) {
			return Result{}, &StepError{StepCreate, fmt.Errorf("retry id %q is not an orca operation id", r.RetryID)}
		}
		args = append(args, "--retry-launch", r.RetryID)
	}
	out, err := o.run(r.Dir, nil, "orca", args...)
	if err != nil {
		return Result{}, &StepError{StepCreate, orcaFailure(out, err)}
	}
	wt, launch, err := parseOrcaCreate(out)
	if err != nil {
		return Result{}, &StepError{StepCreate, err}
	}
	if launch.Handle == "" {
		return Result{}, &StepError{StepStart, fmt.Errorf("orca created workspace %s but did not report an agent launch (%s)", wt.where(), orcaQuote(out))}
	}
	return Result{Where: wt.where(), Branch: wt.Branch}, nil
}

// launchLarge is the fallback for a context too long for --prompt: create the
// workspace, launch the agent in it, then send the text to its terminal.
func (o orcaBackend) launchLarge(r Request) (Result, error) {
	if r.RetryID != "" {
		return Result{}, &StepError{StepCreate, errors.New("a retry id only applies to a launch that sends the prompt in one call, and this context is too long for that")}
	}
	args := append(orcaCreateArgs(r), "--json")
	out, err := o.run(r.Dir, nil, "orca", args...)
	if err != nil {
		return Result{}, &StepError{StepCreate, orcaFailure(out, err)}
	}
	wt, _, err := parseOrcaCreate(out)
	if err != nil {
		return Result{}, &StepError{StepCreate, err}
	}
	res := Result{Where: wt.where(), Branch: wt.Branch}

	args = []string{"agent", "launch", "--agent", r.Agent.OrcaID, "--worktree", "id:" + wt.ID}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, "--json")
	out, err = o.run(r.Dir, nil, "orca", args...)
	if err != nil {
		return Result{}, &StepError{StepStart, fmt.Errorf("workspace %s was created but the agent did not start (do not launch again from scratch, it would make a second workspace): %w", wt.where(), orcaFailure(out, err))}
	}
	handle, err := parseOrcaAgentLaunch(out)
	if err != nil {
		return Result{}, &StepError{StepStart, fmt.Errorf("workspace %s was created: %w", wt.where(), err)}
	}

	out, err = o.run(r.Dir, nil, "orca", "terminal", "send", "--terminal", handle, "--text", r.Context, "--enter", "--json")
	if err != nil {
		return Result{}, &StepError{StepPaste, fmt.Errorf("the agent is running in %s but the context was not sent: %w", wt.where(), orcaFailure(out, err))}
	}
	warn, err := parseOrcaSend(out)
	if err != nil {
		return Result{}, &StepError{StepPaste, fmt.Errorf("the agent is running in %s: %w", wt.where(), err)}
	}
	res.Note = warn
	return res, nil
}

// orcaEnvelope is the wrapper around every `orca ... --json` reply.
type orcaEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

// orcaQuote shortens orca's output for an error message.
func orcaQuote(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 200 {
		s = s[:200]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
		s += "..."
	}
	return fmt.Sprintf("%q", s)
}

// orcaFailure turns a failed orca call into an error. With --json, orca prints
// the failure as JSON on standard output (and nothing on standard error), so
// the message comes from there; err only says "exit status 1". When the reply
// was lost, orca puts the launch's operation id in the error, and the result
// is a *LostReplyError.
//
// Observed with Orca 1.4.238:
//
//	$ orca repo show --repo name:nonexist --json        (exit 1, on stdout)
//	{"id":"...","ok":false,"error":{"code":"repo_not_found",
//	 "message":"repo_not_found"},"_meta":{"runtimeId":"..."}}
//
// A lost launch reply could not be provoked, so its shape comes from Orca's
// CLI source (cli/agent-launch-recovery.js): code "agent_session_operation_unknown",
// message "Orca could not confirm the launch outcome; the agent may already be
// running and the prompt may already be submitted.", and data.operationId, plus
// data.nextSteps text that says "--retry-launch <id>". The id looks like the
// operationId of a successful launch, "1792921284877-3bf7854de9154a3687fdac0818e337a1".
func orcaFailure(out []byte, err error) error {
	msg := err.Error()
	var env orcaEnvelope
	var id string
	if json.Unmarshal(out, &env) == nil && env.Error != nil {
		if env.Error.Message != "" {
			msg = env.Error.Message
			if env.Error.Code != "" && env.Error.Code != env.Error.Message {
				msg = env.Error.Code + ": " + msg
			}
		}
		var data struct {
			OperationID string `json:"operationId"`
		}
		if json.Unmarshal(env.Error.Data, &data) == nil && orcaOperationExact.MatchString(data.OperationID) {
			id = data.OperationID
		}
	}
	if id == "" && env.Error != nil && env.Error.Code == "agent_session_operation_unknown" {
		id = orcaOperationPattern.FindString(err.Error())
	}
	if id != "" {
		return &LostReplyError{OperationID: id, Cause: msg}
	}
	return errors.New(msg)
}

// orcaWorktree is the part of a worktree record the launcher uses.
type orcaWorktree struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Branch      string `json:"branch"`
	DisplayName string `json:"displayName"`
}

func (w orcaWorktree) where() string {
	if w.DisplayName != "" {
		return w.DisplayName
	}
	return w.Path
}

type orcaLaunch struct {
	Handle string
}

// parseOrcaCreate reads the reply of `orca worktree create --json`.
//
// Observed with Orca 1.4.238 for `orca worktree create --name
// gh9s-spike-1792921281 --base-branch origin/main --issue 9999 --agent claude
// --prompt <150 KB> --json` (shortened, other fields omitted):
//
//	{"id":"16fc57ec-...","ok":true,"result":{
//	  "worktree":{"id":"42a91ff8-9692-4cc1-965e-16e8ba8703b3::/Users/me/orca/workspaces/gh-9s/gh9s-spike-1792921281",
//	    "path":"/Users/me/orca/workspaces/gh-9s/gh9s-spike-1792921281",
//	    "branch":"refs/heads/MurabitoB/gh9s-spike-1792921281",
//	    "displayName":"gh9s-spike-1792921281","linkedIssue":9999,
//	    "baseRef":"refs/remotes/origin/main",...},
//	  "lineage":{...},"workspaceLineage":null,
//	  "launch":{"outcome":{"kind":"terminal","handle":"term_8f0d1538-a4b6-43b8-9b55-9056fc57fb93","paneKey":"..."},
//	    "worktreeId":"42a91ff8-...::/Users/me/...","receipt":{"mode":"terminal",...},
//	    "prompt":{"delivery":"submit","outcome":"handed-to-terminal"},
//	    "operationId":"1792921284877-3bf7854de9154a3687fdac0818e337a1"}}}
//
// Orca put its own prefix on the name (branch MurabitoB/gh9s-spike-...), so the
// branch comes from here and not from the name that was asked for. Without
// --agent the reply is the same without "launch". An Orca host older than this
// puts the terminal handle in result.agentTerminalHandle instead of
// launch.outcome.handle; both are accepted.
func parseOrcaCreate(out []byte) (orcaWorktree, orcaLaunch, error) {
	bad := func(why string) (orcaWorktree, orcaLaunch, error) {
		return orcaWorktree{}, orcaLaunch{}, fmt.Errorf("orca printed %s instead of a new workspace (%s)", orcaQuote(out), why)
	}
	var env orcaEnvelope
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		return bad("not a successful JSON reply")
	}
	var res struct {
		Worktree            orcaWorktree `json:"worktree"`
		AgentTerminalHandle string       `json:"agentTerminalHandle"`
		Launch              *struct {
			Outcome struct {
				Handle string `json:"handle"`
			} `json:"outcome"`
		} `json:"launch"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return bad("result is not an object")
	}
	wt := res.Worktree
	if !orcaWorktreeIDPattern.MatchString(wt.ID) {
		return bad("no worktree id")
	}
	wt.Branch = strings.TrimPrefix(wt.Branch, "refs/heads/")
	if wt.Branch == "" || strings.ContainsAny(wt.Branch, " \t\x7f") || strings.IndexFunc(wt.Branch, func(c rune) bool { return c < 0x20 }) >= 0 {
		return bad("no branch")
	}
	var l orcaLaunch
	if res.Launch != nil && orcaTerminalPattern.MatchString(res.Launch.Outcome.Handle) {
		l.Handle = res.Launch.Outcome.Handle
	} else if orcaTerminalPattern.MatchString(res.AgentTerminalHandle) {
		l.Handle = res.AgentTerminalHandle
	}
	return wt, l, nil
}

// parseOrcaAgentLaunch reads the terminal handle from `orca agent launch
// --json`.
//
// Observed with Orca 1.4.238 for `orca agent launch --agent claude --worktree
// id:<worktree id> --json` (shortened):
//
//	{"id":"f0e4dd26-...","ok":true,"result":{"outcome":{"kind":"terminal",
//	  "handle":"term_30a4f63f-acbd-45a4-97ac-27a646f7fb61","paneKey":"..."},
//	  "worktreeId":"42a91ff8-...::/Users/me/...","receipt":{...},
//	  "placement":{"groupId":"..."},"operationId":"1792921346166-09a8e2bd55644f878afce7738e25a469"}}
//
// It can also answer with a chat instead of a terminal (outcome.kind "chat"),
// which has no terminal handle to send text to; that is an error here.
func parseOrcaAgentLaunch(out []byte) (string, error) {
	var env orcaEnvelope
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		return "", fmt.Errorf("orca printed %s instead of an agent launch", orcaQuote(out))
	}
	var res struct {
		Outcome struct {
			Kind   string `json:"kind"`
			Handle string `json:"handle"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil || !orcaTerminalPattern.MatchString(res.Outcome.Handle) {
		return "", fmt.Errorf("orca started the agent as %q without a terminal to send the context to (%s)", res.Outcome.Kind, orcaQuote(out))
	}
	return res.Outcome.Handle, nil
}

// parseOrcaSend checks the reply of `orca terminal send ... --json` and
// returns a note to show the user when Orca could not see the prompt start.
//
// Observed with Orca 1.4.238 for `orca terminal send --terminal <handle> --text
// <2 KB> --enter --json` (shortened):
//
//	{"id":"bd13274f-...","ok":true,"result":{"send":{"handle":"term_30a4f63f-...",
//	  "accepted":true,"bytesWritten":2011,"prompt":{"requestId":"7fbf659a-...",
//	  "stages":["input_accepted"],"provider":"claude","observation":"supported",...}},
//	  "mutation":{"requestId":"7fbf659a-...","replayed":false},
//	  "warnings":["input was accepted but no turn start was observed, so the Enter
//	  may have been swallowed. Confirm delivery by reissuing ..."]}}
//
// The agent did answer that prompt, so the warning is not a failure; it is
// passed on as a note.
func parseOrcaSend(out []byte) (string, error) {
	var env orcaEnvelope
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		return "", fmt.Errorf("orca printed %s instead of a send receipt", orcaQuote(out))
	}
	var res struct {
		Send struct {
			Accepted bool `json:"accepted"`
		} `json:"send"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil || !res.Send.Accepted {
		return "", fmt.Errorf("orca did not accept the context (%s)", orcaQuote(out))
	}
	if len(res.Warnings) > 0 {
		return "orca could not confirm the agent started on the context, check its terminal", nil
	}
	return "", nil
}

// OrcaDefaultBase returns the base ref Orca has set for the repo that dir
// belongs to, or "", false when there is none or it cannot be read.
//
// `orca repo show --repo path:<dir> --json` takes the path of the repo's main
// checkout; for a linked worktree it fails with repo_not_found, so then the
// repo id is looked up from the worktree (`orca worktree show --worktree
// path:<dir> --json`) and `repo show --repo id:<id>` is used instead.
//
// Observed with Orca 1.4.238 (shortened):
//
//	$ orca repo show --repo path:/Users/me/orca/projects/gh-9s --json
//	{"id":"...","ok":true,"result":{"repo":{"id":"42a91ff8-9692-4cc1-965e-16e8ba8703b3",
//	  "path":"/Users/me/orca/projects/gh-9s","displayName":"gh-9s","kind":"git",...}}}
//
// The repo has no base ref set, and then the record has no base-ref field at
// all. Orca's source (the `repo set-base-ref` command, and the settings UI
// reading repo.worktreeBaseRef) names the field "worktreeBaseRef"; that field
// was not seen with a value, because setting one would change the user's Orca
// settings.
func OrcaDefaultBase(run RunFunc, dir string) (string, bool) {
	base, repoOK := orcaRepoBase(run, dir, "path:"+dir)
	if repoOK {
		return base, base != ""
	}
	out, err := run(dir, nil, "orca", "worktree", "show", "--worktree", "path:"+dir, "--json")
	if err != nil {
		return "", false
	}
	var env orcaEnvelope
	if json.Unmarshal(out, &env) != nil || !env.OK {
		return "", false
	}
	var res struct {
		Worktree struct {
			RepoID string `json:"repoId"`
		} `json:"worktree"`
	}
	if json.Unmarshal(env.Result, &res) != nil || !orcaRepoIDPattern.MatchString(res.Worktree.RepoID) {
		return "", false
	}
	base, _ = orcaRepoBase(run, dir, "id:"+res.Worktree.RepoID)
	return base, base != ""
}

// orcaRepoBase asks `orca repo show` for the repo's base ref. repoOK says
// whether orca found the repo at all; base is "" when none is set or the value
// is unusable.
func orcaRepoBase(run RunFunc, dir, selector string) (base string, repoOK bool) {
	out, err := run(dir, nil, "orca", "repo", "show", "--repo", selector, "--json")
	if err != nil {
		return "", false
	}
	var env orcaEnvelope
	if json.Unmarshal(out, &env) != nil || !env.OK {
		return "", false
	}
	var res struct {
		Repo struct {
			ID              string `json:"id"`
			WorktreeBaseRef string `json:"worktreeBaseRef"`
		} `json:"repo"`
	}
	if json.Unmarshal(env.Result, &res) != nil || res.Repo.ID == "" {
		return "", false
	}
	base = strings.TrimSpace(res.Repo.WorktreeBaseRef)
	if rejectArg(base) != "" {
		base = ""
	}
	return base, true
}
