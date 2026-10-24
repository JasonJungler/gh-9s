# Release 4b — the inbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `:inbox` (and `0`) opens one screen with four tabs — **assigned issues** (opens on this), review-requested PRs, my PRs, mentions — built from GitHub searches across every host `gh` is logged in to; `enter` opens an item in the existing PR/issue screens and `esc` returns to the same tab and row.

**Architecture:** `internal/ghcli/inbox.go` builds and parses one GraphQL search per tab per host and merges hosts. `internal/tui/inbox.go` registers a screen (like the diff and issue screens do) that keeps its tabs, rows, "new" marks and per-host notes in its screen data, refreshes itself at most once a minute, and opens items by setting the target repo and number the way the issue and PR lists do.

**Tech Stack:** Go (mise), `gh api graphql`, the screen registry in `internal/tui/screen.go`.

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release4-design.md` ("Part 2").

## Global Constraints

- Data only through `gh` (`gh api graphql --hostname H …`, `gh auth status --json hosts`); no query text is ever built from user-typed input (the `/` filter is applied locally to the rows already loaded). `@me` is resolved with the host's `viewer { login }`.
- At most 4 search queries per host per refresh and at most one refresh a minute (GitHub's search limit is about 30 requests a minute); a manual `R` may refresh at once but not more than once every 5 seconds.
- Each tab shows at most 100 rows, newest update first; when GitHub reports more, the tab says "N more on GitHub".
- A host that fails (offline, expired token, rate limit) gives one line on the screen with `gh`'s own words; the other hosts still show. All hosts failing shows the lines and an empty list, not a crash.
- Results carry the identity (`tabs`, host list) they were fetched for; a late result for anything else is dropped (use `screen.identity`).
- Opening an item uses the item's own repo including its host (not the current directory's, not the base repo); `esc` returns to the inbox with the same tab, row and scroll, and the repo/target the user had before opening the inbox is restored when leaving it.
- Existing keys keep working: global `1`–`4` stay "runs/workflows/pulls/issues", so the inbox switches tabs with `tab` / `shift+tab` and `h` / `l`.
- No network in tests (seams). Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release4b-inbox` off `main`; no push/tag without approval. README updated.

## Review Focus

- A host whose token expired while another works; a host offline; GraphQL returning `data` with `errors` (partial results); rate-limit messages; an empty result; a result with a `null` node (a repository the user lost access to).
- Rows from two hosts with the same `owner/repo#number`: they are different items (identity includes the host) and the host is shown on the Enterprise ones.
- Opening an Enterprise item, then `esc`, then opening a github.com item: the second open must not carry the first one's host.
- Draft PRs, PRs by bots (`author` is null for deleted users), titles with escape sequences or newlines (sanitised), 100+ results, a long title in a narrow terminal.
- The "new" marker: not shown on the very first load, shown for rows that appeared since the previous refresh, cleared when the tab is left; a row that moves tabs is not "new" in both.
- Refresh while a row is selected: the selection stays on the same item (by identity), not the same index.
- The poll: a screen left open for hours makes about one request round per minute, none while the screen is not shown, and a failing host is not hammered (back off to the next minute).
- Switching the repo (`:repo`) while the inbox is open or after leaving it does not change the inbox (it is not about one repo).

---

### Task 1: Hosts, queries and parsing

**Files:**
- Create: `internal/ghcli/inbox.go`, `internal/ghcli/inbox_test.go`

**Interfaces:**
- Produces:
  - `type InboxTab int` with `InboxAssigned, InboxReviewRequested, InboxMine, InboxMentions`
  - `type InboxItem struct { Host, Repo string; Number int; Kind string /* "issue" or "pr" */; Title, Author, URL, UpdatedAt string; Draft bool; ReviewDecision, Checks string }`
  - `func InboxHosts() ([]string, error)` — from `gh auth status --json hosts` (parsed by `ParseAuthHosts([]byte) []string`), falling back to the default host alone when that fails or the gh is too old; github.com first
  - `func InboxSearch(tab InboxTab, login string) string` — the GitHub search string for the tab (`is:issue is:open assignee:LOGIN sort:updated-desc`, `is:pr is:open review-requested:LOGIN archived:false …`, `is:pr is:open author:LOGIN …`, `is:open mentions:LOGIN -author:LOGIN …`)
  - `func InboxArgs(host string, search string) []string` — `api graphql [--hostname H] -f query=<QUERY> -f q=<search> -F n=100`
  - `func ParseInbox(host string, b []byte) (items []InboxItem, total int, err error)` — items sanitised through `CleanText`, null nodes skipped, `checks` ∈ `SUCCESS|FAILURE|PENDING|ERROR|""`, a response with `errors` and no `data` is an error carrying the first message, with `data` it returns the items
  - `func FetchInbox(host string, tab InboxTab, login string) ([]InboxItem, int, error)` and `func FetchLogin(host string) (string, error)` (`viewer { login }`)

- [ ] **Step 1: Write the failing tests:** `ParseAuthHosts` on a real `--json hosts` fixture with two hosts and with an error entry (`"state":"error"` hosts are still listed, marked in the result?), github.com first; `InboxSearch` for the four tabs (exact strings, login embedded only after validation against `^[A-Za-z0-9][A-Za-z0-9-]*(\[bot\])?$`); `InboxArgs` for github.com and an Enterprise host (the query text and `q` travel as separate `-f` arguments); `ParseInbox` fixtures: an issue, a PR with review decision and a check rollup, a draft PR, a null author, a null node, an escape sequence in a title, total > len(items), `errors`+`data` partial, `errors` only → error, empty.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run 'Inbox|AuthHosts'` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/ghcli -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: inbox searches through GraphQL`.

### Task 2: The screen

**Files:**
- Create: `internal/tui/inbox.go`, `internal/tui/inbox_test.go`
- Modify: `internal/tui/model.go` (view kind), `internal/tui/completion.go` (`inbox` command), `internal/tui/keys.go` (`0` key), `internal/tui/view.go` (hint line)

**Interfaces:**
- Consumes: Task 1; seams `inboxHosts = ghcli.InboxHosts`, `inboxLogin = ghcli.FetchLogin`, `inboxFetch = ghcli.FetchInbox`.
- Produces: `viewInbox` registered with `register`; `inboxState { tab int; tabs [4]inboxTab; hostNotes []string; loadedAt time.Time }` where `inboxTab { items []ghcli.InboxItem; total int; cursor, top int; seen map[string]bool; fresh map[string]bool }`; identity = the joined host list.

- [ ] **Step 1: Write the failing tests:** `:inbox` and `0` open it on the assigned-issues tab with the four tab labels and counts; rows show repo, number, title, author, age (and checks/review for PRs; the host on Enterprise rows); `tab`/`shift+tab`/`h`/`l` switch tabs and each keeps its own cursor; two hosts' rows are merged newest first and a failing host gives a note line while the other shows; all hosts failing shows the notes and an empty list with a sentence; an empty tab says "nothing assigned to you" (and the equivalents); more than the cap shows "N more on GitHub"; `/` filters the loaded rows locally; `R` refreshes (throttled), the minute poll refreshes (a fake clock or a counter through the tick path) and not more often; after a refresh the selection stays on the same item; the "new" marks (not on first load, set for newcomers, cleared on leaving the tab); a late result for another host list is dropped; `:repo` does not touch the inbox's contents.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Inbox` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: the inbox screen`.

### Task 3: Open an item and come back

**Files:**
- Modify: `internal/tui/inbox.go`, `internal/tui/keys.go` (`popBack` restores the target saved when the inbox was entered), `README.md`
- Test: `internal/tui/inbox_test.go`

**Interfaces:**
- Consumes: the PR/issue screens' entry state (`m.target`, `m.pr`, `m.prTarget`, `m.issue`, `viewPRDetail`, `viewIssue`).
- Produces: `enter` on a PR row opens `viewPRDetail` for that item's repo (host included); on an issue row `viewIssue`; `o` opens the item's URL in the browser; the inbox remembers the target it was opened from and `esc` from it restores that.

- [ ] **Step 1: Write the failing tests:** enter on a github.com PR then `esc` returns to the same tab, row and scroll with the previous `m.target` restored; the same for an Enterprise issue (the detail fetch is called with the Enterprise repo ref); open Enterprise item, back, open a github.com item: no host carried over; from the PR detail every existing action (`d` diff, `c` checks, `r` review) still acts on the item's repo; `o` opens the URL once through the `openURL` seam; selecting a row that vanished after a refresh does nothing.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Inbox` — Expected: FAIL.
- [ ] **Step 3: Implement**, update the README (the `0` key, the command, the tabs, what the "new" mark means). **Step 4: Run** the suite as in Task 2 — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (open with the base repo; skip the host in the identity of rows; skip restoring the target) and **commit** `feat: open inbox items in the PR and issue screens`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (an issue assigned to them on a test repo appears on the first tab, a PR requesting their review, the "new" mark after a refresh, an Enterprise host if they have one).
