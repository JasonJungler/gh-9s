# Release 3d — inline comments and one-shot review submission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On the review screen, put the cursor on a line (or select a range with `V`), press `c` to write an inline comment; comments pile up as drafts, kept on disk per PR, and go out together with the verdict (approve / request changes / comment) as one GitHub review.

**Changes after looking at similar tools (lgtm, tuicr, octo.nvim) and GitHub's docs:** drafts persist in `drafts.json` next to `favorites.json` (survive quitting/crashes); line ranges are supported (`start_line`/`start_side`); `REQUEST_CHANGES` and `COMMENT` need a non-empty review body (GitHub's documented rule), only `APPROVE` may go without. Unverified assumptions to check on a test PR: that `line`+`side` (not `position`) is accepted, and what GitHub says when the user already has a pending review.

**Architecture:** `internal/ghcli` gains `SubmitReview` (one `gh api …/pulls/N/reviews` POST with a JSON body on stdin, pinned to a commit) and a head-commit check. The review screen gets a line cursor, a pure `rowTarget` that maps a row to (path, line, side), drafts kept in `diffState`, and the existing review flow (verdict choice → body → confirmation) extended to include the drafts.

**Tech Stack:** Go (mise), `gh api`.

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release3-design.md` ("Version 3": inline comments, submitting). Release 3c (reading) is merged; this builds on `diffState`, `sbLayout`, `reviewKey`.

## Global Constraints

- Everything is sent for the repo and PR the screen was opened for (`m.target`, `m.pr`), never `m.base`; stale async results are dropped by identity.
- The review body and every comment travel as JSON on stdin (`gh api --input -`), never inside an argv string or a shell.
- Nothing is sent without the confirmation dialog, which names `owner/repo#N`, the verdict, and the number of inline comments.
- The review is pinned to `commit_id` = the head commit the diff was fetched at. If the PR's head moved since, refuse (message + refetch); never post comments against a different commit.
- Drafts are written to `drafts.json` (per host/repo/PR, with the head commit they were written against) on every change and loaded when the PR's diff opens; leaving the screen with drafts does not ask (nothing is lost), but a successful submit and `X` (discard all, with confirmation) remove them. Single-line and line-range comments (a range stays on one side of one file).
- Existing keys keep working; the new ones are `c` (comment on the line, new side), `C` (old side, when the row has an old line), `V` (start/extend a range; `esc` cancels), `x` (delete the draft under the cursor), `X` (discard all drafts of this PR), `r` (review verdict; also still available on the PR detail).
- Remote text sanitised as everywhere; no network in tests (seams). Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release3d-comments` off `main`; no push/tag without approval. README updated.

## Review Focus

- A comment on a line that GitHub does not accept (outside the diff, a hunk header, a binary file) is refused before sending, with a sentence saying why; a 422 from GitHub shows GitHub's reason.
- An existing pending review of the user (GitHub allows one) → GitHub's message is shown unchanged, drafts are kept, nothing is lost.
- A body or comment with quotes, newlines, emoji, `-`-prefixed text, a 60,000-character body.
- Drafts across a refetch (`R`), a layout switch and a restart: kept when their file and line still exist in the new diff, listed as dropped otherwise; a corrupt or hand-edited `drafts.json` (left untouched, drafts disabled with a status note); same line commented twice; draft deleted; 100 drafts; drafts written for an older head commit shown with a note.
- The cursor: moving with j/k/ctrl+d/g/G keeps it on screen, searching and `]`/`[`/`}`/`{` move it, resizing across 120 columns keeps it on the same source line, it never sits on a blank or truncation row.
- Request changes / comment with an empty body is refused with a sentence (GitHub requires a body); approve with drafts and no body is allowed.
- Submitting twice (double key press) sends one review.

---

### Task 1: Submit a review through the API

**Files:**
- Create: `internal/ghcli/submit.go`, `internal/ghcli/submit_test.go`
- Modify: `internal/ghcli/gh.go` (add `RunInput`)

**Interfaces:**
- Produces:
  - `func RunInput(stdin []byte, args ...string) ([]byte, error)` — like `Run`, with stdin
  - `type InlineComment struct { Path string; Line int; Side string /* LEFT|RIGHT */; StartLine int /* 0 for one line */; Body string }`
  - `func ReviewPayload(head string, kind ReviewKind, body string, comments []InlineComment) ([]byte, error)` — JSON `{commit_id, event: APPROVE|REQUEST_CHANGES|COMMENT, body (omitted when empty), comments: [{path, line, side, body}]}`; refuses request-changes and comment with an empty body (GitHub requires one), and any comment with an empty body, empty path, line < 1, or StartLine >= Line (`start_line`/`start_side` are sent only for ranges)
  - `func SubmitReviewArgs(r RepoRef, number string) []string` — `api [--hostname H] --method POST repos/OWNER/NAME/pulls/N/reviews --input -`
  - `func SubmitReview(r RepoRef, number, head string, kind ReviewKind, body string, comments []InlineComment) error`
  - `func PRHead(r RepoRef, number string) (string, error)` — `gh pr view N --json headRefOid`

- [ ] **Step 1: Write the failing tests:** `ReviewPayload` for approve/no comments (no `comments` key, no `body` key), comment with drafts and a tricky body (`"quotes"`, newline, `-x`, emoji, 60,000 chars) round-trips through `json.Unmarshal` unchanged; request-changes with nothing is an error; empty comment body / line 0 / empty path are errors; `SubmitReviewArgs` for github.com and for an Enterprise host.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run 'Payload|SubmitArgs'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/ghcli -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: submit a review with inline comments through the API`.

### Task 2: A line cursor and what it points at

**Files:**
- Create: `internal/tui/cursor.go`, `internal/tui/cursor_test.go`
- Modify: `internal/tui/diffview.go`, `internal/tui/review.go` (keys, drawing of the cursor row, footer)

**Interfaces:**
- Produces:
  - `type target struct { Path string; Line int; Side string }`
  - `func rowTarget(st diffState, row int, oldSide bool) (target, string)` — the comment target for a display row in the current layout (unified or side-by-side) and, when there is none, a sentence saying why ("hunk headers can't be commented on", "binary file", …); Add/Context rows → RIGHT with the new line number; Del rows → LEFT with the old number; `oldSide` picks the old line of a context row or a changed pair
  - `diffState.cur int` — the cursor's display row; kept visible by the keys

- [ ] **Step 1: Write the failing tests:** table over unified and side-by-side layouts of a fixture diff — for each kind of row the target (or the refusal sentence); moving with j/k, ctrl+d/u, g/G, `}`/`{`, `]`/`[`, search jumps keeps `cur` within `[top, top+height)`; the cursor row is drawn marked (a `▶`/reverse gutter) in both layouts; the cursor skips blank and note rows; after a layout switch the cursor is on the same source line.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Cursor|RowTarget'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Existing scroll tests keep their meaning: `m.top` stays the scroll position; only add the cursor and make scrolling keys drag it along.
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: a line cursor on the review screen`.

### Task 3: Drafts

**Files:**
- Create: `internal/tui/drafts.go`, `internal/tui/drafts_test.go`
- Modify: `internal/tui/diffview.go`, `internal/tui/review.go`, `internal/tui/model.go` (leave-screen guard)

**Interfaces:**
- Consumes: Task 2 `rowTarget`, `newTextDialog`.
- Produces: `type draft struct { target; Body string }`; `diffState.drafts []draft` (loaded from and saved to `internal/drafts`, a new package modelled on `internal/recent`: `Load(path)`, `Get(host, repo, number)`, `Put(host, repo, number, head, []Draft)`, `Delete(...)`, validating everything it reads); `c`/`C` open a text dialog titled `Comment on path:line (new|old) of owner/repo#N`; on OK the draft is added; drafts show in place under their line (reuse `threadRows` through a `ghcli.Thread` with author `you (pending)`) and in the footer as `N drafts`; `x` removes the draft under the cursor; `reconcileDrafts(old []draft, d diff.Diff) (kept, dropped []draft)` is applied on every refetch (dropped ones are listed in the status line); `V` starts a range at the cursor and moving extends it (shown highlighted), `c` then comments the whole range, `esc` cancels; `X` discards all drafts of the PR after a confirm dialog.

- [ ] **Step 1: Write the failing tests:** `c` on a changed line opens the dialog naming the file, line, side and PR, OK adds the draft, it appears under the line in both layouts; `C` on a deleted line uses the LEFT side; two drafts on one line both show; `x` removes only the one under the cursor; `c` on a hunk header only sets the status sentence; empty text adds nothing; refetch keeps drafts whose line still exists and reports dropped ones; `V`, move, `c` makes a range draft shown under its last line; a range across files or sides is refused; a restart (new model, same store) brings the drafts back; `X` + `y` clears them from memory and disk; drafts of another PR never appear.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Draft` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** the suite as in Task 2 — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: draft inline comments`.

### Task 4: Send them with the verdict

**Files:**
- Modify: `internal/tui/pr_actions.go` (review flow), `internal/tui/diffview.go` (`r` key), `README.md`
- Test: `internal/tui/review_submit_test.go`

**Interfaces:**
- Consumes: Task 1 `SubmitReview`, `PRHead`, `InlineComment`; Task 3 `drafts`.
- Produces: seams `submitReview = ghcli.SubmitReview`, `prHead = ghcli.PRHead`; `prAction` gains `drafts []draft` and `head string`.

- [ ] **Step 1: Write the failing tests:** `r` on the diff screen with drafts → verdict dialog → (body dialog for request-changes/comment) → confirmation that names `owner/repo#N`, the verdict and "with N inline comments"; `y` calls `prHead` and, if it equals the diff's head, `submitReview` once with the drafts mapped to `InlineComment` and the repo/PR the screen was opened for (not `m.base`); success clears the drafts and shows `✓ …`; a moved head refuses with a sentence and refetches, keeping the drafts; a GitHub error (422, pending review exists) is shown as is and the drafts stay; request-changes with no body and no drafts is refused; with no drafts the old `gh pr review` path is used unchanged; pressing `y` twice sends one review (the second press finds no pending action).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'ReviewSubmit'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (send drafts of another PR; skip the head check; skip clearing drafts) and **commit** `feat: send inline comments with the review`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (a PR of their own on a test repo: comment on an added line, a deleted line, a context line; approve; request changes with drafts and no body; an existing pending review on the web).
