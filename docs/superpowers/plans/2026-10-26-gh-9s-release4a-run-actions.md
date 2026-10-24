# Release 4a — safer, fuller run actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `r` (re-run), `F` (re-run failed jobs), `c` (cancel) and `d` (delete) on the run list ask for confirmation naming `owner/repo` and the run, explain when they don't apply, and also work on the job graph and the job list; an accepted re-run lands on that run's graph and is watched again; the graph can jump to the first failure.

**Architecture:** A pure rule function says whether an action applies to a run in a given status. A small `runAct` (the action, repo, run id, label) travels through the existing dialog machinery like `prAction` does. `ghcli` gets typed functions for the four actions instead of ad-hoc `gh` argument lists in `keys.go`.

**Tech Stack:** Go (mise), existing dialog/confirm and graph code.

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release4-design.md` ("Part 1").

## Global Constraints

- Every action acts on the repo and run the screen was opened for, captured when the key is pressed (`m.target`, the run's id), never `m.base` and never "whatever is selected when the dialog is answered"; navigating away before answering drops the action.
- A confirmation names `owner/repo` (use `displayRef(m.curTarget())`) and the run id; nothing runs without it.
- Existing keys keep working; the graph's `x` (collapse) stays as it is. New letters on the graph are the builder's choice and recorded in the README; `r`, `F`, `c`, `d` are free on the graph and the job list.
- No network in tests (seams). Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release4a-runs` off `main` (after the design branch is merged); no push/tag without approval. README updated.

## Review Focus

- Pressing `r`/`F`/`c`/`d` twice quickly, or answering a dialog after the list refreshed and the selected row changed: still only the run the dialog named is touched.
- A run that finished between the key press and the answer: `gh` says so, and its message is shown unchanged.
- `F` on a run whose failed jobs were already re-run (the attempt number moved); `r` on a run older than GitHub allows; no permission.
- After a re-run the graph for the same run id starts fresh (old nodes gone), polls again although `runStatus` was `completed`, and the finished notification fires once for the new attempt, not at once for the old status.
- The job list and the graph are reached from a PR's checks too: the action must use the run's own repo (which can differ from the PR's), as the check jump already does.
- Enterprise: `-R HOST/OWNER/REPO` is passed whenever the target has a host.

---

### Task 1: Rules and typed actions

**Files:**
- Create: `internal/ghcli/runs.go`, `internal/ghcli/runs_test.go`, `internal/tui/runacts.go`, `internal/tui/runacts_test.go`
- Modify: `internal/tui/keys.go` (remove the ad-hoc `r`/`F`/`c`/`d` cases once Task 2 wires the new ones)

**Interfaces:**
- Produces (ghcli):
  - `type RunAction string` with `RunRerun = "rerun"`, `RunRerunFailed = "rerun-failed"`, `RunCancel = "cancel"`, `RunDelete = "delete"`
  - `func RunActionArgs(r RepoRef, id string, a RunAction) []string` — `run rerun ID [--failed] -R …`, `run cancel ID -R …`, `run delete ID -R …`
  - `func DoRunAction(r RepoRef, id string, a RunAction) error`
- Produces (tui):
  - `func runRule(a ghcli.RunAction, status, conclusion string) (ok bool, why string)` — rerun: refused while `status != "completed"` ("this run is still in progress"); rerun-failed: refused unless the conclusion is one of failure, timed_out, cancelled-with-failed-jobs-unknown… keep it simple: allowed when `completed` and conclusion != "success" ("nothing failed to re-run" on success; "still in progress" otherwise); cancel: only for status queued/in_progress/waiting/pending/requested ("this run already finished"); delete: only completed ("cancel it first")
  - `type runAct struct { action ghcli.RunAction; target ghcli.RepoRef; id, label string }` and `seam doRunAction = ghcli.DoRunAction`

- [ ] **Step 1: Write the failing tests:** `RunActionArgs` for each action with and without a host (`-R HOST/OWNER/REPO`); `runRule` table (every action × queued, in_progress, completed/success, completed/failure, completed/cancelled, completed/timed_out), exact refusal sentences; status values in any case (`IN_PROGRESS`).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli ./internal/tui -run 'RunAction|RunRule'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** the same — Expected: PASS.
- [ ] **Step 5: Commit** `feat: typed run actions and the rules for when they apply`.

### Task 2: Confirmed actions on the run list

**Files:**
- Modify: `internal/tui/keys.go` (the run list keys), `internal/tui/runacts.go`, `internal/tui/model.go` (field `pendingRun *runAct`), `internal/tui/update.go` (done message), rows in `internal/tui/tables.go` (carry the run's status and conclusion, e.g. in `row.extra` or two new fields)
- Test: `internal/tui/runacts_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces: dialog handler id `run.confirm`; `runActDoneMsg{text string, err error, rerun bool, target ghcli.RepoRef, id string}`.

- [ ] **Step 1: Write the failing tests:** `r` on a failed run opens a confirmation whose title names `acme/api` and the run id, nothing is run before `y`, `n`/esc run nothing; `y` calls the seam once with exactly that repo/id/action even if the cursor moved or the list was refreshed before the answer; `F` on a successful run gives the status sentence and no dialog; `r` on a running run likewise; `c` on a finished run likewise; `d` keeps working with the new dialog; the status line shows `✓ re-ran …` or `✗ <gh's message>`; after a successful re-run the model is on that run's graph (`viewGraph`, `runID` set, `target` kept, graph state reset, `runStatus` cleared so the poll restarts and the finished notification re-arms); pressing `y` twice runs once; the old immediate behaviour is gone (no seam call without the dialog).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'RunAct'` — Expected: FAIL.
- [ ] **Step 3: Implement** — rows carry status/conclusion; the four keys open the confirmation through `newConfirmDialog`; the handler runs `doRunAction` as a `tea.Cmd` and returns `runActDoneMsg`; `update.go` handles it (on a re-run: reset the graph state the same way `triggeredMsg` does and `push(viewGraph)`).
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: confirmed re-run, cancel and delete on the run list`.

### Task 3: The same actions on the graph and the job list, and jump to the failure

**Files:**
- Modify: `internal/tui/keys.go` (`graphKey`, the job list key path), `internal/tui/runacts.go`, `internal/tui/cmds.go` (open a job's log with a position)
- Test: `internal/tui/runacts_test.go`, `internal/tui/graphview_test.go`

**Interfaces:**
- Produces: `func (m model) runActFor(a ghcli.RunAction) (model, tea.Cmd)` shared by the three screens (uses `m.runID`, `m.target` and `m.run`/`m.runStatus` for the rules); a graph key `f` ("first failure") that opens the log of the first failed job in the graph, with the view scrolled to its first line containing `##[error]`, else the first match of `(?i)error|failed`, else the top.

- [ ] **Step 1: Write the failing tests:** on the graph and on the job list `r`, `F`, `c`, `d` behave as on the run list (same dialog, same rules, same seam calls), using the run shown (a run reached from a PR's check in another repo acts on that repo); `x` still collapses a group; `f` with a failed job selects the first failed job in graph order and opens its log scrolled to the `##[error]` line; with no failed job it says so; a log without markers falls back to the regex, then to the top; keys that were already handled on those screens still are.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'RunAct|FirstFailure'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** the suite as in Task 2 — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (act on the selected row at answer time instead of key-press time; skip the confirmation; skip the graph reset after a re-run) and **commit** `feat: run actions on the graph and job list, jump to the first failure`.

### Task 4: README and manual check

- [ ] **Step 1:** Update the key table in `README.md` (run list, graph, job list: `r F c d`, graph `f`, the confirmation behaviour).
- [ ] **Step 2: Commit** `docs: run actions`.
- [ ] **Step 3: Manual check by the user:** list in the final message what to try (re-run failed jobs on a red run of a repo they own, cancel a running one, the graph key `f` on a failed run).
