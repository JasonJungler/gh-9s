# Release 5d — the preview pane and the gh-dash look Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The PR list, the issue list and the inbox (every tab) show a pane on the right with the selected item's details, like gh-dash: loaded after a short pause, cached, scrollable on its own, shown when the terminal is wide enough and the settings allow it.

**Architecture:** A layout helper splits the body into a list column and a pane column from the `preview*` settings. A preview cache (identity → detail, with a time and a fetch sequence) is filled by debounced fetches through the existing `fetchPR`/`fetchIssue` seams. The pane renders PR/issue details with the existing markdown renderer. Rounded borders and a full-width selection highlight apply to these lists for everyone.

**Tech Stack:** Go (mise), lipgloss borders, the settings store (5a), icons (5b).

**Spec:** `docs/superpowers/specs/2026-10-27-gh-9s-release5-design.md` ("Part 4").

## Global Constraints

- Shown when `preview` is on and the terminal has at least `preview_min_columns` columns; width is `preview_width` percent of the screen (list keeps the rest, never less than 50 columns; if that cannot hold the pane is not shown). The pane never changes what the list can do; below the threshold the lists are full width as today. A key toggles the pane for the session (builder picks the key from the free ones and puts it in the footer); the setting changes it for good.
- Data only from the existing detail fetches (`fetchPR`, `fetchIssue`); the failing checks' names come from the `statusCheckRollup` data `FetchPR` already requests (add the check name to what is kept).
- Debounce: the fetch for a newly selected row starts after 250 ms of the selection not changing (a sequence number drops the earlier ticks); the cache holds at most 50 entries (least recently used out), an entry is reused for 5 minutes and re-fetched earlier when the list row's `updatedAt` is newer than the cached detail's; a result for a row that is no longer selected is cached but not shown.
- Identity: host + repo + number + kind; two hosts' `acme/api#1` are different entries; an inbox row opens its own repo's detail (host included) for the preview too, never the current directory's.
- The pane scrolls independently (keys chosen by the builder, shown in the footer), shows "loading…" while fetching and the error text on failure; text from GitHub is sanitised (`safe.Text` via the markdown renderer and `CleanText`); images show as markers, as in the detail screens.
- Existing keys keep working; `enter` still opens the full screen. Existing view tests are rewritten where the layout changed. No network in tests (seams); no test touches the user's config. Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release5d-preview` off `main`; no push/tag without approval. README updated.

## Review Focus

- Holding `j` on a 50-row list: the number of requests (one per pause, none per row), results arriving out of order, the selection moving while a fetch is in flight, a fetch failing then the user moving away and back.
- Resizing across `preview_min_columns` and `preview_width` changes in `:settings` while the pane is open: the list keeps its cursor, the pane keeps its scroll or resets sensibly; a 40-column terminal; a pane taller than the screen.
- The inbox: tabs switching with the pane open (the pane follows the new tab's selection), Enterprise rows, rows that vanish after a refresh, an empty tab (pane shows nothing rather than stale content).
- PR with a 60,000-character body, 200 comments, 500 checks, a bot author, a deleted author, escape sequences everywhere; markdown render time on the UI goroutine (use the render cache; cap what the pane renders).
- Icons on and off: borders and highlight identical; the pane's header icons only in the Nerd set.
- Cache growth and memory after hours open; the debounce timer leaking when the screen is left.

---

### Task 1: Layout, borders and the full-width highlight

**Files:**
- Create: `internal/tui/panes.go`, `internal/tui/panes_test.go`
- Modify: `internal/tui/view.go` (table body for lists with a pane), `internal/tui/inbox.go`, `internal/tui/styles.go`

**Interfaces:**
- Produces: `func (m model) previewShown() bool` (setting, width threshold, current view in {PRs, issues, inbox}, session toggle); `func (m model) paneWidths() (list, pane int)`; `func joinPanes(left, right string, lw, rw, h int) string` (rounded borders around both, exact widths and heights); the selected row highlighted across the list's full width.

- [ ] **Step 1: Write the failing tests:** widths for 80/100/119/120/160/200 columns and several `preview_width` values (list ≥ 50, pane ≥ 30, sum exactly the terminal width, nothing when it cannot fit); `joinPanes` with CJK, ANSI-coloured and over-long lines produces exactly `h` lines of exactly the total width; the toggle key flips it for the session only; below the threshold the list output equals today's (full-width, no borders) — old tests unchanged; the selected row is highlighted to the list's edge.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Pane|Preview'` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** the suite — Expected: all `ok`. **Step 5: Commit** `feat: lists split into a list and a pane`.

### Task 2: Preview data — selection, debounce, cache

**Files:**
- Create: `internal/tui/previewdata.go`, `internal/tui/previewdata_test.go`
- Modify: `internal/ghcli/pr.go` (check names in `PRDetail`), `internal/ghcli/pr_test.go`

**Interfaces:**
- Produces: `type previewKey struct{ host, repo, kind string; number int }`; `type previewEntry struct{ pr *ghcli.PRDetail; issue *ghcli.IssueDetail; err error; at time.Time }`; model state with `previewSel previewKey`, `previewSeq int`, the cache; `previewTickMsg{seq int, key previewKey}`; `previewLoadedMsg{key previewKey, entry previewEntry}`; seams `previewNow`, `previewDelay`; `PRDetail.FailingChecks []string` (names of failed/timed-out/cancelled checks, deduplicated, capped at 20).

- [ ] **Step 1: Write the failing tests:** moving the selection five times within the delay produces one fetch for the last row; a tick with an old sequence number does nothing; a loaded result for an earlier row is cached and not shown; reselecting a cached row within 5 minutes shows it without a fetch, after 5 minutes (fake clock) or when the row's `updatedAt` is newer it fetches again; the 51st entry evicts the least recently used; an error is cached for 30 seconds only; Enterprise and github.com rows with the same repo/number are different keys; leaving the screen cancels pending ticks' effect; `FailingChecks` parsing (names, dedupe, cap, sanitised).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui ./internal/ghcli -run 'Preview|FailingChecks'` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** the suite — Expected: all `ok`. **Step 5: Commit** `feat: preview data with a debounce and a cache`.

### Task 3: The panes' content and scrolling

**Files:**
- Create: `internal/tui/previewview.go`, `internal/tui/previewview_test.go`
- Modify: `internal/tui/panes.go`, `internal/tui/prdetail.go` (share the building blocks), `README.md`

**Interfaces:**
- Produces: `func previewLines(e previewEntry, width int) []string` (PR: title, number and state/draft icon, author, `base ← head`, labels, review decision and checks with the failing ones named, size, description as markdown, latest comments; issue: title, state, author, labels, assignees, description, latest comments); the pane's own scroll state and keys.

- [ ] **Step 1: Write the failing tests:** a PR entry renders every field above in plain and nerd mode (icons only in nerd); markdown (headings, task list, code block, image markers) renders as in the detail screen; 500 checks and 200 comments are capped with "N more"; escape sequences and bidi in every field are removed; scrolling keys move the pane and clamp; "loading…" and error text show; the pane content for a row whose entry changes while scrolled resets the scroll; rendering a 60,000-character body takes under 200 ms and is cached per (entry, width).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Preview` — Expected: FAIL. **Step 3: Implement**; document the pane, the toggle and scroll keys, and the three settings in the README. **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=2'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (fetch per keypress; show a result for a row that is no longer selected; key entries without the host) and **commit** `feat: the preview pane shows the selected PR or issue`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (the PR list, the issue list and the inbox at 120+ columns; holding `j`; the toggle key; `:settings` width; a 100-column terminal has no pane).
