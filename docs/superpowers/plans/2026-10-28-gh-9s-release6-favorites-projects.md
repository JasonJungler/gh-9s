# Release 6 — favorites in tabs: workflows, repos, GitHub Projects (read-only) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `:fav` opens a screen with tabs Workflows / Repos / Projects; repos and GitHub Projects can be pinned; a pinned Project opens as a read-only board of its issues and PRs grouped by Status.

**Architecture:** A new `internal/pins` package stores pinned repos and projects (`pins.json`). `internal/ghcli/project.go` reads a Project with one GraphQL query per page through `gh api graphql`. The TUI gets a tab bar on the favorites screen (today's table becomes its Workflows tab), a Repos tab, a Projects tab, and a new board screen (`viewBoard`) built like the inbox (tabs, rows, open into the PR/issue screens, preview pane).

**Tech Stack:** Go (mise), Bubble Tea v1, lipgloss, the `gh` CLI, icons (5b), preview pane (5d).

**Spec:** `docs/superpowers/specs/2026-10-28-gh-9s-release6-design.md`.

## Global Constraints

- Read-only: no mutating GraphQL or `gh project` write command exists anywhere in the change; a test greps the new code for `mutation`, `item-edit`, `item-add`, `item-create`, `item-delete`, `item-archive`.
- Only `internal/ghcli` runs `gh`; Enterprise hosts via `--hostname`; remote text through `CleanText`; names from `pins.json` validated like favorites (plain tokens `^[A-Za-z0-9][A-Za-z0-9._-]*$`, project number a positive integer, no leading `-`) before reaching any command line.
- `pins.json` (next to `favorites.json`; `GH9S_PINS` overrides the path) is `{"repos":[{"host","repo"}],"projects":[{"host","owner","number","title"}]}`; atomic write, mode 0600, re-read before write; an unparsable file is never overwritten: the screen says so and pins live in memory for the session only. `favorites.json` keeps its array format.
- Board: first Status tab is `All`, then the Project's Status options in the Project's order, then `No status` when needed; at most 200 items (two pages of 100, sequential); if `totalCount` is larger, counts read `N+` and the screen says how many more exist; not polled; refetch on `R` (not within 5 s of the last) and on re-entry after more than 60 s.
- Errors: `missing required scopes [read:project]` (or similar) → `Projects need the read:project permission. Run: gh auth refresh -s read:project` (add `--hostname HOST` for a non-github.com host); 502/503/504 retried once (`withRetry` from prchecks.go); other errors shown sanitised; pins stay.
- Stale async results are dropped by identity (project key, tab, request id); actions use identity captured at key-press time. A draft item has no repo: `enter` says so, `o` opens the Project.
- The Workflows tab behaves exactly as the old favorites screen (its tests stay unchanged); the last tab used is remembered for the session only; `:fav` and the other existing entry points open the tabbed screen.
- No test touches the network or the user's config (seams; `GH9S_PINS` to a temp dir). TDD. Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release6-favorites` off `main`; no push/tag without approval. README and `?` help (`help.go`, whose footer-vs-help test must keep passing) updated.

## Review Focus

- A Project with 0 items, only items without Status, a Status option with no items, 300 items (cap + `N+`), a draft issue, an item whose repo is on another host or was deleted (content null / inaccessible), emoji/CJK/escape sequences in titles and Status names, 100 Status options.
- Pins: duplicates (same repo twice, case differences), a hand-edited file with a hostile value (`--owner`, newline, `../`), a broken file, a missing directory, two gh-9s instances writing.
- Tab switching while a board request is in flight, switching to another Project/repo, pressing `R` repeatedly, `esc` from board to the right tab of favorites, resize across the preview threshold with the board open.
- `enter` on a pinned repo that no longer exists or is private to another account; `:fav repo` when no repo is current; `ctrl+f` in the picker on an empty list.
- Missing-scope detection must not fire on unrelated errors; a token with scope but a Project owned by an org that restricts tokens (SAML) shows that error text, not the scope hint.

---

### Task 1: The pins store

**Files:**
- Create: `internal/pins/pins.go`, `internal/pins/pins_test.go`

**Interfaces:**
- Produces:
  - `type Repo struct{ Host, Repo string }` (json `host`, `repo`), `type Project struct{ Host, Owner string; Number int; Title string }` (json `host`, `owner`, `number`, `title`)
  - `type Store struct{ Repos []Repo; Projects []Project; ... }`; `func DefaultPath() string` (honours `GH9S_PINS`); `func Load(path string) (*Store, error)` (missing/empty file → empty store; unparsable or invalid entry → error, file untouched); `func (s *Store) Save() error`; `func (s *Store) ToggleRepo(r Repo) (added bool)`; `func (s *Store) ToggleProject(p Project) (added bool)`; `func (s *Store) HasRepo(r Repo) bool`; `func (s *Store) HasProject(host, owner string, n int) bool`; `func ParseRepo(s string) (Repo, error)` (`[HOST/]OWNER/NAME`); `func ParseProject(s string) (host, owner string, n int, err error)` (`[HOST/]OWNER/NUMBER`); `func (r Repo) Target() ghcli.RepoRef`

- [ ] **Step 1: Write the failing tests:** round trip of both lists; order of pinning kept; duplicates (host `""` equals `github.com`; owner/name compared case-insensitively) toggle off instead of duplicating; `ParseRepo`/`ParseProject` accept `acme/api`, `ghe.corp/acme/api`, `acme/3`, `ghe.corp/acme/3` and reject empty parts, `-x/y`, `a/b/c/d`, number 0, negative, `3x`, newline, `../x`; `Load` rejects a hostile entry and a non-JSON file and does not change the file on disk; missing directory is created on `Save`; file mode is 0600 and no `.tmp` is left; `GH9S_PINS` overrides `DefaultPath`; `Save` re-reads the file first so a pin added by another instance is not lost (two stores, interleaved).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/pins` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** the declarations above in `internal/pins/pins.go` (mirror `internal/favorites` for validation and atomic writes).
- [ ] **Step 4: Run** `mise exec -- go test ./internal/pins -race -count=2` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: the store of pinned repos and projects`.

### Task 2: Reading a Project

**Files:**
- Create: `internal/ghcli/project.go`, `internal/ghcli/project_test.go`

**Interfaces:**
- Produces:
  - `type ProjectItem struct{ Status string; Type string /* "Issue" "PullRequest" "DraftIssue" or "" for no access */; Number int; Title, URL, Repo string; State string /* OPEN CLOSED MERGED */; Draft bool; Updated string; Assignees, Labels []string }`
  - `type Project struct{ Title string; Closed bool; StatusOrder []string; Total int; Items []ProjectItem }`
  - `var ErrMissingScope = errors.New("missing read:project permission")`
  - `func ParseProjectPage(b []byte) (p Project, hasNext bool, cursor string, err error)` — input is the `gh api graphql` output; a null `projectV2` is an error "project not found or no access"; `content: null` items become `Type: ""` rows titled "(no access)"; a PR with `merged` has `State: "MERGED"`; text through `CleanText`
  - `func FetchProject(host, owner string, number int) (Project, error)` — up to two sequential pages of 100 (`gh api graphql [--hostname H] -F owner=… -F number=… -f query=…`, cursor passed as `-F after=…`), `ErrMissingScope` (wrapped with the stderr text) when gh reports missing scopes, one retry on 502/503/504/timeouts per page; the query is the one in the spec (read-only, no `mutation`)
  - `func ScopeHint(host string) string` — the `gh auth refresh -s read:project[ --hostname HOST]` sentence of the spec

- [ ] **Step 1: Write the failing tests** (`ParseProjectPage` on fixtures and `FetchProject` with the stub-gh pattern of the other ghcli tests): full page with an issue, an open/merged/draft/closed PR, a draft issue, an item with `content: null`, an item without Status; status order from the field options; page 1 `hasNextPage` → second call carries the cursor and results concatenate, total 250 gives `Total: 250` with 200 items; a null project → error; missing-scope stderr → `errors.Is(err, ErrMissingScope)` while a SAML/403 error is not; 504 on the first try then success → one retry; non-github.com host adds `--hostname`; owner/number are passed as variables, never pasted into the query text; escape sequences in title/status/label are removed; ScopeHint with and without host; a grep-style test that the query text contains no `mutation`.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run Project` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/ghcli -race -count=2` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: read a GitHub Project (items by status) through the GraphQL API`.

### Task 3: The tabbed favorites screen and the Repos tab

**Files:**
- Create: `internal/tui/favtabs.go`, `internal/tui/favtabs_test.go`
- Modify: `internal/tui/model.go` (pins store loaded at start like favorites; `favTab int`), `internal/tui/favview.go` / `keys.go` / `view.go` (tab bar and key routing for `viewFavs`), `internal/tui/reposwitch.go` / picker (pinned marks, `ctrl+f`), `internal/tui/help.go`, `README.md`

**Interfaces:**
- Consumes: Task 1 `pins.Store`. Produces: `model.pins *pins.Store` (nil with a startup note when the file is broken → session-only store), `model.favTab` (0 Workflows, 1 Repos, 2 Projects; session memory), tab bar `Workflows (n) | Repos (n) | Projects (n)`; Repos tab rows `[HOST/]OWNER/NAME`; keys on the Repos tab: `enter` = same as `:repo OWNER/NAME`, `a` pins (text dialog, validated by `pins.ParseRepo` and by the existing repo check used by `:repo`), `f`/`d` unpin after a confirm dialog, `o` opens in the browser; `:fav repo [REPO]` pins the current (or given) repo; in the repo picker pinned repos carry a mark and `ctrl+f` toggles the pin of the selected row.

- [ ] **Step 1: Write the failing tests:** `:fav` shows the tab bar with counts and starts on Workflows, and every existing favorites test still passes unchanged; `tab`/`shift+tab`/`h`/`l` cycle tabs and the tab is remembered when leaving and re-entering; Repos tab lists pins in order (host shown only for non-github.com); `a` + text pins and saves `pins.json` (temp path), an invalid name shows an error and pins nothing; `f` asks first, `esc` keeps the pin, `y` removes and saves; `enter` switches repo exactly like `:repo` (same `target`, same stack reset); `:fav repo` with no current repo says so; the picker shows the mark and `ctrl+f` toggles and saves; broken `pins.json` → banner, pins work for the session, file untouched; the tab bar and rows keep exact widths with CJK names at 60–200 columns; `?` help lists the tabs' keys and the footer-vs-help test passes.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Fav|Pin'` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=2'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: favorites in tabs, with pinned repos`.

### Task 4: Projects tab and the board

**Files:**
- Create: `internal/tui/board.go`, `internal/tui/board_test.go`
- Modify: `internal/tui/favtabs.go` (Projects tab), `internal/tui/model.go` (`viewBoard`, `board` target), `internal/tui/previewdata.go` (board rows as preview targets), `internal/tui/help.go`, `README.md`

**Interfaces:**
- Consumes: Tasks 1–3, `ghcli.FetchProject`, `ghcli.ErrMissingScope`, `ghcli.ScopeHint` (seam `fetchProject = ghcli.FetchProject` in the tui package). Produces: Projects tab rows `title  [HOST/]OWNER/#N  open|closed`; keys `enter` (open the board), `a` (text dialog `OWNER/NUMBER`, validated by `pins.ParseProject`, then the Project is read once to get and store its title; the scope hint on `ErrMissingScope`), `f`/`d` (unpin after a confirm), `o` (browser); `viewBoard` registered like the inbox (screen registry): title `<project title>  OWNER/#N`, tab bar `All (n) | <Status> (n)… | No status (n)`, rows `icon  REPO #N  title  assignees  labels  updated`, `tab/shift+tab/h/l` switch tabs, `enter` opens the item in the PR or issue screen of its own repo and host (as inbox rows do), draft items say they cannot be opened, `o` browser, `/` filter, `R` refresh (5 s throttle), `esc` back to the Projects tab; board rows work with the preview pane (issue/PR only).

- [ ] **Step 1: Write the failing tests:** the Projects tab lists pins; `a` with `acme/3` reads the Project (fake `fetchProject`), stores its title, saves; unknown project → error, nothing pinned; missing scope → the hint sentence with `--hostname` for an Enterprise host and the pins/tabs still work; `enter` opens the board with tabs in Status order, counts, `All` first, `No status` only when needed, an empty Status option shows `(0)`; 250 items → `N+` counts and the "more" sentence; rows show state icons (open/closed/merged/draft) in the Nerd set and the plain symbols otherwise, widths exact; `enter` on an issue/PR opens the right screen with the item's own repo/host (Enterprise item), on a draft item shows the message and stays; `o` on a draft opens the Project URL; tab switch while a request is in flight drops the late answer; a request for another Project is dropped; `R` twice within 5 s fetches once; re-entry after 60 s refetches, within 60 s does not; `esc` returns to favorites on the Projects tab; the preview pane follows board rows (issue/PR) and shows nothing for a draft item; 504 then success shows the board; a grep test that no write command or `mutation` appears in non-test sources of the change; `?` help for the board and the footer-vs-help test.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Board|Project'` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=2'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (key one tab's items into another tab; let a late response for a previous Project replace the board; drop the host from the open-item target) and **commit** `feat: pinned GitHub Projects open as a read-only board`.
- [ ] **Step 6: Manual check by the user:** `gh auth refresh -s read:project` first; then `:fav` → Projects → `a` → `OWNER/NUMBER` of a real Project; browse tabs, open an issue and a PR, `o`, `R`; on an Enterprise host if available; and pin a repo (`:fav repo`) and `enter` on it.
