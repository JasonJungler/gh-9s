# Release 3c — side-by-side review screen (reading) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The diff screen becomes a review screen: file list on the left, side-by-side diff with word-level highlights on the right (single column when narrow), per-file "viewed" marks, hunk navigation, and other people's inline review comments shown under their lines. Writing inline comments and batch submission is release 3d, built on this.

**Architecture:** `internal/diff` gains line numbering and a word-level diff (pure). `internal/ghcli` gains `FetchReviewThreads`. A new `tui/sidebyside.go` lays a file out in two panes; `diffview.go` keeps its screen registration, search and cap, and swaps its layout for the new one.

**Tech Stack:** Go (mise), lipgloss/ansi already in the module.

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release3-design.md` ("Version 3"; this plan is its reading half).

## Global Constraints

- Remote text sanitised (`safe.Text`) before display; diff text already is, review comments go through `markdown.Render` like the other detail screens (images in them are plain markers, not selectable here).
- Actions/fetches use the repo and PR the screen was opened for (`m.target`, `m.pr`); stale async results are dropped by identity (`screen.identity`).
- Existing keys keep working: `j/k`, `ctrl+d/u`, `g/G`, `]`/`[` next/prev file, `f` file list, `/` search with `n/N`, `o` browser, `esc`. New keys: `tab` switch pane, `s` side-by-side/unified, `v` viewed, `}` / `{` next/previous hunk.
- Side-by-side needs at least 120 columns; narrower terminals get the existing unified view automatically, `s` overrides either way for the session.
- "Viewed" marks are kept in memory per (repo, PR, head SHA); a new head SHA clears them.
- The cap `maxDiffLines` is raised from 5,000 to 50,000 lines; beyond it a file is marked truncated as today.
- No network in tests (seams); run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release3c-review` off `main`; no push/tag without approval. README updated.

## Review Focus

- Binary files, renames, a file with only deletions or only additions, an empty diff, a file with a 10,000-char line, CRLF files, tabs, wide (CJK/emoji) characters: columns stay aligned and no line exceeds its pane.
- Unified→side-by-side pairing of unequal runs (3 deletions followed by 1 addition and the reverse) leaves blanks on the short side and pairs the right lines for word highlighting.
- Word highlight on lines that differ completely, on identical-but-for-whitespace lines, on very long lines (must stay fast, linear-ish: cap the word-diff to lines under 2,000 runes).
- Review threads whose line no longer exists (outdated), replies, deleted-line (LEFT side) comments, comments on a file that is not in the diff, thousands of comments (pagination).
- Resizing the terminal across the 120-column threshold keeps the scroll position on the same diff line.
- Search (`/`) still finds text in both panes and `n/N` moves between matches.

---

### Task 1: Line numbers and word diff (pure)

**Files:**
- Create: `internal/diff/number.go`, `internal/diff/words.go`; tests `number_test.go`, `words_test.go`

**Interfaces:**
- Produces:
  - `type NumLine struct { Line; Old, New int }` (0 when the line has no number on that side)
  - `func (f File) Numbered() []NumLine` — numbers from each hunk header `@@ -a,b +c,d @@`; context advances both, Add only New, Del only Old; Hunk/Meta lines carry 0/0
  - `type Span struct{ Start, End int }` (rune offsets)
  - `func WordDiff(old, new string) (oldSpans, newSpans []Span)` — tokens are runs of letters/digits, single other characters; common tokens are not highlighted; inputs over 2,000 runes return one span covering the whole differing middle (common prefix/suffix trimmed)
  - `type Row struct { Old, New *NumLine }` and `func (f File) Rows() []Row` — side-by-side pairing: within a hunk, a run of Del lines followed by a run of Add lines is paired index by index (the short side gets nil), context lines appear on both sides, Hunk/Meta lines span (both pointers set to the same line)

- [ ] **Step 1: Write the failing tests:** `Numbered` on a two-hunk file gives exactly the expected old/new numbers (including a hunk starting at line 1 and a `@@ -0,0 +1,3 @@` new file); `WordDiff("for i := 0; i < 3; i++ {", "for i := 0; i < n; i++ {")` highlights only `3` / `n`; identical strings give no spans; totally different strings highlight everything; whitespace-only change highlights the whitespace; CJK text works by runes; a 5,000-rune pair returns in under 50 ms; `Rows` for 3 Del + 1 Add gives 3 rows (first paired, two with `New == nil`) and for 1 Del + 3 Add the mirror; an Add-only hunk has `Old == nil` throughout; context is on both sides with matching numbers.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/diff` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** the four declarations.
- [ ] **Step 4: Run** `mise exec -- go test ./internal/diff -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: line numbers, side-by-side rows and word diff`.

### Task 2: Other people's review comments

**Files:**
- Create: `internal/ghcli/threads.go`, `internal/ghcli/threads_test.go`

**Interfaces:**
- Produces:
  - `type ThreadComment struct { Author, Body, CreatedAt string }`
  - `type Thread struct { Path string; Line int; Side string /* "LEFT" or "RIGHT" */; Outdated bool; Comments []ThreadComment }`
  - `func ParseThreads(b []byte) ([]Thread, error)` — input is the JSON array of `GET /repos/{o}/{r}/pulls/{n}/comments` (several pages are concatenated by `gh api --paginate --slurp`, so accept an array of arrays too); replies (`in_reply_to_id`) join their parent's thread in time order; a comment whose `line` is null is `Outdated` and uses `original_line`; text goes through `CleanText`
  - `func FetchReviewThreads(r RepoRef, number string) ([]Thread, error)` — `gh api --paginate --slurp [--hostname H] repos/OWNER/NAME/pulls/N/comments`

- [ ] **Step 1: Write the failing tests** (`ParseThreads` on fixtures): one comment → one thread with path/line/side; reply joins its parent; null `line` → `Outdated` with the original line; RIGHT is the default side; escape sequences in body/author are removed; array-of-arrays input; empty array → none; malformed JSON → error. A `FetchReviewThreads` test with the stub-gh pattern used by the other ghcli tests checks the argument list, including `--hostname` for a non-github.com host.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run Thread` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/ghcli -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: fetch review comment threads`.

### Task 3: Side-by-side layout

**Files:**
- Create: `internal/tui/sidebyside.go`, `internal/tui/sidebyside_test.go`

**Interfaces:**
- Consumes: Task 1 `Rows`, `WordDiff`, `NumLine`; Task 2 `Thread`.
- Produces: `type sbLine struct { left, right string; kind dkind; thread bool }` (display strings already styled, each pane exactly its width, padded with spaces) and `func sideBySide(f diff.File, threads []ghcli.Thread, width int) []sbLine` (two panes of `(width-1)/2` columns with a `│` between them, line numbers in a gutter of the width of the largest number; changed words highlighted with reverse video on top of the add/del colour; long lines cut with `…` in their pane; threads for `f.Path` printed under the line they belong to — RIGHT comments in the right pane at the new-side line, LEFT in the left pane — as `💬 author: first line…` followed by the rest wrapped, outdated ones dimmed with "(outdated)" at the end of the file).

- [ ] **Step 1: Write the failing tests** (assert on `ansi.Strip`ped text): a modified line shows old on the left and new on the right with their own numbers; an added-only line has an empty left pane; a 10,000-char line stays within the pane and ends in `…`; CJK and tab content keep the separator in the same column on every row; binary file gives a single note row; a RIGHT thread appears directly below its line and a LEFT thread under the deleted line; an outdated thread is at the end marked `(outdated)`; every `left`/`right` has display width exactly its pane width (property over several fixtures and widths 120, 121, 200).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run SideBySide` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** the same — Expected: PASS. **Step 5: Commit** `feat: side-by-side diff layout`.

### Task 4: The review screen

**Files:**
- Modify: `internal/tui/diffview.go` (state, `diffDisplay`, keys, body, footer), `internal/tui/prdetail.go` only if the PR's `HeadSHA` must be passed on, `README.md`
- Test: `internal/tui/review_test.go` (existing `diffview_test.go` keeps passing)

**Interfaces:**
- Consumes: Tasks 1–3; `fetchThreads = ghcli.FetchReviewThreads` (new seam next to `fetchDiff`).
- Produces: `diffState` gains `threads []ghcli.Thread`, `viewed map[string]bool` (by file path), `sbs int` (0 auto, 1 forced side-by-side, 2 forced unified), `focusList bool`, `head string`; the fetch returns the diff and the threads together (a threads failure shows a status note "couldn't load review comments: …" but still shows the diff).

- [ ] **Step 1: Write the failing tests:** at width 140 the screen shows a file list column and `│`-separated panes, at width 100 the existing unified view; `s` toggles and sticks across resizes; `tab` moves focus between list and diff and `j/k` act on the focused pane, `enter` in the list jumps the diff to that file; `v` marks the current file (✓ in the list) and `v` again clears it; marks survive a refetch with the same head SHA and clear with a new one; `}`/`{` move to the next/previous hunk; the file list shows `+n −m` and a 💬 count for files with threads; threads are fetched for the PR the screen was opened for (target, not base) and a late result for another PR is dropped; a failing thread fetch still shows the diff and a status note; `/` search finds text on both sides and `n/N` still work; resizing across 120 keeps the same diff line at the top; 20,000-line diff stays under the cap and is marked truncated at 50,000.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Review|Diff'` — Expected: FAIL.
- [ ] **Step 3: Implement** — keep `m.logs` as the search corpus (both panes' plain text joined per row), keep `m.top` as the scroll position in display rows, raise `maxDiffLines` to 50,000 and update the existing truncation test accordingly.
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (e.g. send threads of another PR to the screen; drop the head-SHA reset of `viewed`) and **commit** `feat: review screen with side-by-side diff, viewed marks and existing comments`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (a real PR with review comments, a wide and a narrow window, a rename, a binary file).
