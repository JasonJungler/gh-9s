# Send an Issue to a Coding Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One key on an issue opens a one-line confirmation; `enter` opens a workspace (Orca) or pane (herdr, tmux), starts Claude or Devin there with a chosen model, and gives it the issue as context.

**Architecture:** A new `internal/launch` package holds everything that is not UI: environment detection, the agent table (with model lists), the context text, and three backends behind one `Launcher`. Orca uses its own `worktree create --agent --prompt`; herdr and tmux share one open-pane → start-agent → wait-ready → paste sequence over a small `space` interface. `internal/tui` adds one overlay (the confirmation line that expands into a form) and the key on both issue screens. Every external program runs through one injectable `RunFunc`, so no test launches anything.

**Tech Stack:** Go (module `github.com/MurabitoB/gh-9s`), Bubble Tea, the `orca`, `herdr`, `tmux`, `git`, `claude`, `devin` CLIs. Run tests with `mise exec -- go test ./...`.

**Spec:** `docs/superpowers/specs/2026-10-25-send-issue-to-agent-design.md`

## Global Constraints

- The context text starts with the fixed sentence `請解決這個 issue`; then title, body and **all** comments. Same for every agent and environment.
- Environment priority when several are detected: **orca, then herdr, then tmux**; setting `launch_target` (auto | orca | herdr | tmux, default auto) overrides.
- Detection variables: Orca `ORCA_WORKTREE_ID`; herdr `HERDR_ENV` or `HERDR_PANE_ID`; tmux `TMUX`.
- Settings (all Enum): `launch_target` auto|orca|herdr|tmux (auto); `launch_agent` claude|devin (claude); `launch_tmux_place` pane|window (pane); `launch_split` horizontal|vertical (horizontal). herdr uses the same place/split settings (herdr "window" = tab).
- Claude's model list: `default`, `best`, `fable`, `opus`, `sonnet`, `haiku`, `opusplan`, `sonnet[1m]`, `opus[1m]`. Devin's list comes from `devin models list --format json`; on failure fall back to `opus`, `sonnet`, `swe`, `codex`, `gemini`. Free text is always accepted. `default` passes no model flag.
- tmux readiness: poll the new pane's output for the agent's ready marker, **30 s** timeout, no setting; on timeout paste nothing and leave the pane open.
- Orca call: `orca worktree create --name <name> --base-branch <base> --issue <N> --agent <id> [--model <id>] --prompt <context> --json`, run from the current directory. Orca adds its own prefix to the branch, so show the branch from the result.
- Context over **100 000** characters: warn on the confirmation line, still send.
- Only `internal/ghcli` runs `git`; only `internal/launch` runs `orca`/`herdr`/`tmux`/agents.
- Nothing is created before `enter` on the confirmation; a failed step is named; nothing is cleaned up or retried silently.
- Issues only; no branch handling in tmux/herdr; no effort flag; no live API model list.

## Review Focus

- A model name, branch title or comment containing spaces, quotes, `;`, `$(…)` or backticks is never executed by the shell tmux types into (Task 4).
- An issue with an empty body, zero comments, a deleted-user comment (`ghost`), CJK/emoji text, or over 100 000 characters still produces valid context (Tasks 1, 8).
- tmux started inside an Orca terminal picks Orca; no environment at all gives a clear message instead of a crash (Task 1).
- Context is only ever sent to the pane just created: every command after "open" names that pane id, and a failed open runs nothing further (Tasks 4, 5).
- A failing `git fetch` stops an Orca launch before anything is created, and a repo with no remote shows a visible note; viewing a different repo than the working directory's refuses to launch (Tasks 3, 6, 9).

---

### Task 1: Core types, detection, context and branch name

**Files:**
- Create: `internal/launch/launch.go`, `internal/launch/context.go`
- Test: `internal/launch/launch_test.go`, `internal/launch/context_test.go`

**Interfaces:**
- Produces: `type Target string` with `TargetOrca = "orca"`, `TargetHerdr = "herdr"`, `TargetTmux = "tmux"`; `type Place string` (`PlacePane = "pane"`, `PlaceWindow = "window"`); `type Split string` (`SplitHorizontal = "horizontal"`, `SplitVertical = "vertical"`).
- Produces: `func Detect(getenv func(string) string, override string) (Target, error)`; `var ErrNoEnvironment = errors.New("no supported environment found (orca, herdr or tmux)")`.
- Produces: `func BuildContext(d ghcli.IssueDetail) string`; `const WarnChars = 100000`; `func BranchName(number int, title string) string`.

- [ ] **Step 1: Write failing tests.** `TestDetectPriority` (env with `ORCA_WORKTREE_ID`+`TMUX` → orca; `HERDR_PANE_ID`+`TMUX` → herdr; `TMUX` → tmux), `TestDetectOverride` (override `"tmux"` with orca env → tmux; `"auto"` and `""` detect), `TestDetectNone` (→ `ErrNoEnvironment`). `TestBuildContext`: fixture `{Number:42, Title:"Fix login", URL:"https://x/42", Body:"It crashes.", Comments:[{Author:"ann",Body:"Seen too"},{Author:"",Body:"me3"}]}` equals exactly `"請解決這個 issue\n\n# Fix login (#42)\nhttps://x/42\n\nIt crashes.\n\n## Comments\n### ann (1/2)\nSeen too\n\n### ghost (2/2)\nme3\n"`; empty body → line `(no description)`; zero comments → no `## Comments` section. `TestBranchName`: `(42,"Fix login crash!")` → `issue-42-fix-login-crash`; empty title → `issue-42`; title of 200 chars → name ≤ 50 chars with no trailing `-`; CJK-only title → `issue-42`.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/launch/ -v` — expect FAIL (package missing).
- [ ] **Step 3: Implement** the signatures above. Slug: lowercase ASCII letters/digits, runs of anything else become one `-`, trimmed, whole name capped at 50.
- [ ] **Step 4: Run** the same command — expect PASS.
- [ ] **Step 5: Commit** `feat: launch package core (detect, context, branch name)`.

---

### Task 2: Agent table and model lists

**Files:**
- Create: `internal/launch/agents.go`, `internal/launch/run.go`
- Test: `internal/launch/agents_test.go`

**Interfaces:**
- Produces: `type RunFunc func(dir string, stdin []byte, name string, args ...string) ([]byte, error)`; `func ExecRun(dir string, stdin []byte, name string, args ...string) ([]byte, error)` (os/exec; error text = trimmed stderr when present).
- Produces: `type Agent struct { ID, Label, OrcaID, HerdrKind, TmuxCommand, ModelFlag, ReadyMarker string; Verified bool }`; `var Agents = []Agent{…claude, devin}`; `func LookupAgent(id string) (Agent, bool)`.
- Produces: `type Model struct{ ID, Hint string }` (ID `""` is "default"); `func ListModels(a Agent, run RunFunc) []Model` — always starts with the default entry.

- [ ] **Step 1: Write failing tests.** `TestClaudeModels`: IDs after default equal the Claude list in Global Constraints, in order, and `run` is never called. `TestDevinModelsLive`: fake `run` returns the JSON the real command prints (Step 3 records it) → models parsed, default first. `TestDevinModelsFallback`: `run` errors; and again with unparseable output → the five fallback aliases, default first.
- [ ] **Step 2: Run** `go test ./internal/launch/ -run Models` — FAIL.
- [ ] **Step 3: Record the real shapes.** Claude is installed: start it in a throwaway tmux (`tmux -L gh9s-probe new-session -d 'claude'`, `capture-pane -p`) and record a stable substring of its ready screen as `ReadyMarker`. Devin is not installed here: take `devin models list --format json`'s shape from docs.devin.ai/cli/reference/commands; leave `Verified:false` and a comment saying the marker and JSON shape are unconfirmed. Kill the probe server afterwards.
- [ ] **Step 4: Implement** the table (`OrcaID`/`HerdrKind` are `claude`/`devin`; `ModelFlag` is `--model`; `TmuxCommand` is the binary name) and `ListModels` (Devin: run `devin models list --format json` with a 5 s cap; any error or empty parse → fallback list).
- [ ] **Step 5: Run** `go test ./internal/launch/` — PASS. **Step 6: Commit** `feat: launch agent table and model lists`.

---

### Task 3: `git fetch` of the base in ghcli

**Files:**
- Create: `internal/ghcli/fetch.go`
- Test: `internal/ghcli/fetch_test.go`

**Interfaces:**
- Produces: `func FetchBase(dir, base string) (note string, err error)` — `base` is `<remote>/<branch>` or a bare branch.

- [ ] **Step 1: Write failing tests** against real temp repos (`git init --bare` as remote, a clone with `origin`): `TestFetchBaseUpdates` (a commit pushed from a second clone becomes visible as `origin/main` after `FetchBase(dir, "origin/main")`, note `""`); `TestFetchBaseBareBranchUsesOrigin` (`"main"` fetches `origin`); `TestFetchBaseNoRemote` (repo with no remote → no error, note `"no remote, using the local branch"`); `TestFetchBaseFailure` (remote path removed → error mentioning the git message).
- [ ] **Step 2: Run** `go test ./internal/ghcli/ -run FetchBase` — FAIL.
- [ ] **Step 3: Implement** `FetchBase`: list remotes with `git -C dir remote`; no remotes → note; a leading segment matching a remote name is that remote, else `origin` when it exists; run `git -C dir fetch <remote>`.
- [ ] **Step 4: Run** — PASS. **Step 5: Commit** `feat: fetch the base ref before launching`.

---

### Task 4: Shared open → start → wait → paste sequence, and tmux

**Files:**
- Create: `internal/launch/sequence.go`, `internal/launch/tmux.go`
- Test: `internal/launch/sequence_test.go`, `internal/launch/tmux_test.go`

**Interfaces:**
- Consumes: Task 1 types, Task 2 `Agent`, `RunFunc`.
- Produces: `type Request struct { Target Target; Agent Agent; Model, Context, Dir string; Place Place; Split Split; Base, Name string; Issue int }`.
- Produces: `type space interface { open(r Request) (pane string, err error); start(pane string, r Request) error; ready(pane string, r Request, timeout time.Duration) error; paste(pane, text string) error }`.
- Produces: `func runSequence(sp space, r Request, timeout time.Duration) error` returning `*StepError`; `type StepError struct { Step string; Err error }` (`Error()` is `"<Step>: <Err>"`); step constants `StepFetch="fetch"`, `StepCreate="create workspace"`, `StepOpen="open pane"`, `StepStart="start agent"`, `StepReady="agent not ready"`, `StepPaste="paste"`.
- Produces: `func shellQuote(s string) string`; `func startCommand(a Agent, model string) string`; `func newTmux(run RunFunc) space`.

- [ ] **Step 1: Write failing tests.** Sequence with a fake `space`: `TestSequenceOrder` (open, start, ready, paste in that order); `TestSequenceStopsOnFailure` (table: failure at each stage → `StepError.Step` is the matching constant and later stages are not called); `TestReadyTimeoutPastesNothing`.
- [ ] **Step 2: tmux tests with a recording fake `RunFunc`.** `TestTmuxOpenPaneHorizontal` (args exactly `split-window -h -P -F #{pane_id} -c <dir>`; vertical uses `-v`; window uses `new-window -P -F #{pane_id} -c <dir>`; pane id is the trimmed stdout); `TestTmuxEveryLaterCommandNamesThePane` (after `open` returned `%7`, every recorded call from `start`, `ready`, `paste` contains `-t %7`; and if `open` errors, only that one call was recorded); `TestTmuxStartTypesQuotedCommand` (`startCommand(claude,"opus")` is `claude --model 'opus'`; model `x'; rm -rf ~ #` is quoted so the single quote is escaped as `'\''`; empty model → just `claude`); `TestTmuxReadyPollsCaptureUntilMarker` (fake returns marker on third `capture-pane`); `TestTmuxPasteUsesBuffer` (calls `load-buffer -b <name> -` with the text on stdin, then `paste-buffer -p -d -b <name> -t %7`, then `send-keys -t %7 Enter`).
- [ ] **Step 3: Run** `go test ./internal/launch/ -run 'Sequence|Tmux'` — FAIL.
- [ ] **Step 4: Implement.** `shellQuote` wraps in single quotes. tmux `start` types `startCommand` with `send-keys -t <pane> -l` then `send-keys -t <pane> Enter`. `ready` polls `capture-pane -p -t <pane>` every 500 ms until `Agent.ReadyMarker` appears or the timeout passes.
- [ ] **Step 5: Verify against the real tmux** in a throwaway server (`tmux -L gh9s-probe`): run a one-off program calling the tmux space with `echo` standing in for the agent, confirm the text arrives in the new pane only, then `tmux -L gh9s-probe kill-server`.
- [ ] **Step 6: Run** `go test ./internal/launch/` — PASS. **Step 7: Commit** `feat: launch sequence and tmux backend`.

---

### Task 5: herdr backend

**Files:**
- Create: `internal/launch/herdr.go`
- Test: `internal/launch/herdr_test.go`

**Interfaces:**
- Consumes: Task 4 `space`, `runSequence`, `RunFunc`.
- Produces: `func newHerdr(run RunFunc, getenv func(string) string) space`.

- [ ] **Step 1: Pin the real command output.** herdr is installed: in a throwaway session (`herdr --session gh9s-probe`), run `herdr pane split --pane <id> --direction right --cwd <dir>`, `herdr tab create --cwd <dir> --label x`, `herdr agent list`, and see what each prints (pane id, JSON shape) and what `herdr agent prompt`'s `<TARGET>` takes (agent name or pane id). Write the findings as comments above the parser.
- [ ] **Step 2: Write failing tests** with a fake `RunFunc` returning those real outputs: `TestHerdrOpenPaneSplit` (`horizontal` → `--direction right`, `vertical` → `--direction down`, `--pane` is `HERDR_PANE_ID`, `--cwd` is `Dir`); `TestHerdrOpenWindowUsesTab`; `TestHerdrStartPassesModel` (`herdr agent start <name> --kind claude --pane <id> -- --model opus`; no `--` when model is empty); `TestHerdrPasteUsesAgentPrompt`; `TestHerdrEveryLaterCommandNamesThePane` (same rule as tmux).
- [ ] **Step 3: Run** `go test ./internal/launch/ -run Herdr` — FAIL.
- [ ] **Step 4: Implement.** `ready` is a no-op because `agent start` already waits for readiness (default 30 s; pass `--timeout 30000`).
- [ ] **Step 5: Run** — PASS. **Step 6: Commit** `feat: herdr backend`.

---

### Task 6: Orca backend and the Launcher

**Files:**
- Create: `internal/launch/orca.go`, `internal/launch/launcher.go`
- Test: `internal/launch/orca_test.go`, `internal/launch/launcher_test.go`

**Interfaces:**
- Consumes: Tasks 1, 2, 4, 5.
- Produces: `type Result struct { Where, Branch, Note string }`; `type Launcher struct { Run RunFunc; Getenv func(string) string; Fetch func(dir, base string) (string, error) }`; `func (l Launcher) Do(r Request) (Result, error)`; `const orcaPromptMax = …` (set in Step 1); `func OrcaDefaultBase(run RunFunc, dir string) (string, bool)`.

- [ ] **Step 1: Spike Orca** (it creates real worktrees, so clean up). From this repo run `orca worktree create --name gh9s-spike --agent claude --prompt <150 KB of multi-line CJK text> --json`, check whether the agent received it intact, then `orca worktree rm` the spike worktree. Set `orcaPromptMax` to the largest size that worked (cap 128 KiB). Also run `orca repo show --repo path:<dir> --json` and note the field that holds the base ref; and `orca terminal send --help` for the fallback.
- [ ] **Step 2: Write failing tests** (fake `RunFunc`): `TestOrcaCreateArgs` (exact argv in the Global Constraints order, `--model` only when set, run in `Dir`); `TestOrcaReadsBranchFromResult` (fake JSON with `branch: refs/heads/MurabitoB/issue-42` → `Result.Branch` is `MurabitoB/issue-42`); `TestOrcaLargeContextFallsBack` (context longer than `orcaPromptMax` → `worktree create` without `--agent`/`--prompt`, then `agent launch`, then `terminal send` with the text); `TestOrcaLostReplyOffersRetry` (error containing an operation id → `StepError` whose `Err` mentions `--retry-launch <id>`, and `Request.RetryID` set on the second call adds that flag); `TestLauncherFetchBeforeCreate` (Orca: fetch called first with `(Dir, Base)`; fetch error → `StepError{Step:StepFetch}` and **no** orca call recorded; fetch note copied into `Result.Note`); `TestLauncherSkipsFetchForTmux`; `TestLauncherPicksBackendByTarget`; `TestOrcaDefaultBase` (field from Step 1 → returned; missing/errors → `"", false`).
- [ ] **Step 3: Run** `go test ./internal/launch/ -run 'Orca|Launcher'` — FAIL.
- [ ] **Step 4: Implement.** Add `RetryID string` to `Request`. `Launcher.Do` dispatches on `r.Target` (orca → `orcaBackend`; herdr/tmux → `runSequence` over their spaces with a 30 s ready timeout).
- [ ] **Step 5: Run** `go test ./internal/launch/` — PASS. **Step 6: Commit** `feat: orca backend and launcher`.

---

### Task 7: Settings and remembered models

**Files:**
- Modify: `internal/settings/registry.go` (append four `Def`s)
- Create: `internal/launch/prefs.go`
- Test: `internal/settings/settings_test.go` (extend), `internal/launch/prefs_test.go`

**Interfaces:**
- Produces: `type Prefs`; `func LoadPrefs(path string) (*Prefs, error)`; `func (p *Prefs) Model(agentID string) string`; `func (p *Prefs) SetModel(agentID, model string)`; `func (p *Prefs) Save() error`.

- [ ] **Step 1: Write failing tests.** Settings: `TestLaunchSettingsDefaults` asserts the four keys, options and defaults from Global Constraints via `settings.Lookup` and `Memory().String`. Prefs: `TestPrefsRoundTrip`, `TestPrefsMissingFileIsEmpty`, `TestPrefsCorruptFileKeepsDefaultsAndReports` (never overwrite a file it could not read, as the settings store does).
- [ ] **Step 2: Run** `go test ./internal/settings/ ./internal/launch/ -run 'Launch|Prefs'` — FAIL.
- [ ] **Step 3: Implement** the `Def`s (`Kind: Enum`, `Label`/`Desc` in the style of the existing ones) and `Prefs` on the same JSON-file pattern as `internal/recent`.
- [ ] **Step 4: Run** — PASS. **Step 5: Commit** `feat: launch settings and remembered models`.

---

### Task 8: The launch dialog (pure state machine and view)

**Files:**
- Create: `internal/tui/launchdialog.go`
- Test: `internal/tui/launchdialog_test.go`

**Interfaces:**
- Consumes: `launch.Target/Place/Split/Model/Agent`, `launch.WarnChars`.
- Produces: `type launchDialog struct{…}`; `func newLaunchDialog(in launchInput) launchDialog` with `type launchInput struct { Issue ghcli.IssueDetail; Target launch.Target; Agent string; Model string; Models []launch.Model; Base, Name string; Place launch.Place; Split launch.Split; ContextLen int }`; `func (d launchDialog) update(k tea.KeyMsg) (launchDialog, launchAct)` where `launchAct` is `actNone|actCancel|actSubmit|actAgentChanged`; `func (d launchDialog) summary() string`; `func (d launchDialog) view(width int) string`; `func (d launchDialog) request() launch.Request`; `d.withModels([]launch.Model)`, `d.withBase(string)`, `d.running()`, `d.failed(err error)`.

- [ ] **Step 1: Write failing tests.** `TestSummaryOrca` equals `orca ◆ new workspace · claude · default model · origin/main → issue-42-fix-login` for matching input; tmux summary is `tmux ◆ new pane (right) · claude · opus` (window → `new window`). `TestEnterSubmitsCompact`; `TestEscCancels`; `TestEExpandsAndTabCyclesApplicableFields` (Base and Branch skipped for tmux/herdr; Place/Split skipped for Orca; Split skipped when Place is window). `TestAgentCycleReloadsModels` (←/→ on Agent returns `actAgentChanged`). `TestModelFieldCycleAndFreeText` (←/→ cycles the list with `default` first; typing replaces with the typed text; empty text means default). `TestWarnsAbove100000` (summary line contains a size warning when `ContextLen > launch.WarnChars`, and submit still works). `TestEnterIgnoredWhileRunning` (second `enter` during `running()` returns `actNone`). `TestFailedShowsStepAndKeepsDialog` (view contains the `StepError.Step`; `enter` retries). `TestFreeTextModelHint` (view next to the model field says a mistyped name only fails in the agent pane).
- [ ] **Step 2: Run** `go test ./internal/tui/ -run 'Launch'` — FAIL.
- [ ] **Step 3: Implement** with the existing `ui` styles and `boxed` look (see `dialog.go`, `form.go`). `request()` builds the `launch.Request` from the fields.
- [ ] **Step 4: Run** — PASS. **Step 5: Commit** `feat: launch confirmation dialog`.

---

### Task 9: Wire it into the issue screens

**Files:**
- Create: `internal/tui/launch.go`
- Modify: `internal/tui/model.go` (field `launchDlg *launchDialog`, `launchPrefs`), `internal/tui/keys.go` (route keys like `m.form`), `internal/tui/view.go` (overlay like `m.form`), `internal/tui/mouse.go:29` (treat the dialog as an overlay), `internal/tui/issues.go` (key `a` in `issuesKey` and `issueKey`, footers), `internal/tui/update.go` (new messages)
- Test: `internal/tui/launch_test.go`

**Interfaces:**
- Consumes: Task 6 `Launcher`, Task 7 `Prefs` and settings, Task 8 dialog, Task 2 `ListModels`, Task 3 `ghcli.FetchBase`.
- Produces: seams `var launcher = launch.Launcher{Run: launch.ExecRun, Getenv: os.Getenv, Fetch: ghcli.FetchBase}`, `var launchModels = launch.ListModels`; messages `launchIssueMsg{id string; d ghcli.IssueDetail; err error}`, `launchModelsMsg{agent string; models []launch.Model}`, `launchBaseMsg{base string}`, `launchDoneMsg{res launch.Result; err error}`.

- [ ] **Step 1: Write failing tests** with `launcher` replaced by a fake: `TestAKeyOnDetailOpensDialog` (issue loaded → dialog open with summary); `TestAKeyOnListFetchesIssueFirst` (cursor row id → `fetchIssue` called, then dialog); `TestRefusesOtherRepo` (`m.target != m.curRepo` → status explains, no dialog, nothing run); `TestNoEnvironmentMessage` (`Detect` fails → status shows the `ErrNoEnvironment` text, no dialog); `TestEnterRunsLauncherOnceAndClosesOnSuccess` (fake `Do` receives the request; status `launched … <branch>`; the chosen agent is written to setting `launch_agent` and the model to `Prefs`); `TestFailureKeepsDialogWithStep`; `TestRetryReusesOperationId` (after a failure carrying an Orca operation id, the next `enter` sends a request with `RetryID` set to it, and a different edit of any field clears it); `TestSettingsSupplyDefaults` (`launch_tmux_place=window`, `launch_split=vertical` reach the request); `TestDialogSwallowsKeys` (no other screen key acts while open).
- [ ] **Step 2: Run** `go test ./internal/tui/ -run Launch` — FAIL.
- [ ] **Step 3: Implement.** Key is `a` on both screens (free there; the shared `a` only acts on workflows). On open: detect target (`launch_target`), read agent/model defaults, build branch name and context length, show the dialog immediately, then start two background commands: `launchModels` for the agent and, for Orca, `launch.OrcaDefaultBase` (fallback `origin/<default branch>` from `gh repo view --json defaultBranchRef`). Submit runs `launcher.Do` in a `tea.Cmd` that returns `launchDoneMsg`; while it runs the dialog shows "launching…".
- [ ] **Step 4: Run** `mise exec -- go test ./...` — everything passes.
- [ ] **Step 5: Smoke test** in a throwaway tmux (`tmux -L gh9s-probe`): build with `mise exec -- go build -o gh-9s .`, run `gh 9s`, press `a` on an issue with a real claude, confirm a new pane opens, claude starts, the context is pasted, and nothing was sent to the original pane. Kill the probe server.
- [ ] **Step 6: Commit** `feat: send an issue to a coding agent with the a key`.

---

### Task 10: Documentation

**Files:**
- Create: `website/src/content/docs/guides/send-to-agent.md`
- Modify: `website/astro.config.mjs` (sidebar entry after Issues), `website/src/content/docs/guides/issues.md` (key rows), `website/src/content/docs/reference/keys.md` (Issues list and Issue detail), `website/src/content/docs/guides/settings.md` (four settings), `README.md` (one feature bullet)

- [ ] **Step 1: Write the guide**: what `a` does, the one-line confirmation and `e`, the three environments and how one is chosen, what Orca does differently (workspace, fetch, branch), the model list and why it is curated, and the failure messages. Frontmatter like the other guides (`title`, `description`).
- [ ] **Step 2: Update the other pages** with the key and the four settings (names and defaults from Global Constraints).
- [ ] **Step 3: Build the site** — `cd website && npm run build` — expect success with no broken-link warnings for the new page.
- [ ] **Step 4: Commit** `docs: guide for sending an issue to an agent`.
