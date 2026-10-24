# Release 5b — Nerd Font icons and the first-run question Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One table of icons with two sets (`plain` = exactly today's symbols, `nerd` = Nerd Font glyphs in gh-dash's style), every place that draws a state symbol reads from it, the `icons` setting chooses the set, and the first start (or `:icons`) asks whether the user can see the icons.

**Architecture:** A new package `internal/icons` holds the table and the active set (process-wide, switched with `icons.Use`, safe for concurrent reads). `ui.RunState`, `ui.Symbol`, the graph renderer, the table builders in `internal/tui` and the new views take their symbols from it. `internal/tui` applies the `icons` setting at start and whenever it changes, and shows the question as a full-screen modal before any other screen.

**Tech Stack:** Go (mise), the settings store from release 5a.

**Spec:** `docs/superpowers/specs/2026-10-27-gh-9s-release5-design.md` ("Part 2").

## Global Constraints

- The `plain` set is exactly what the screens show today; every existing test keeps passing unchanged in plain mode (that is the guard).
- Icons are single-column characters: every glyph in the `nerd` set is one `rune`, in the Basic Multilingual Plane (no supplementary-plane code points), and `ansi.StringWidth` of each is 1 (a test pins this for the whole table). Cells are `icon + " " + text`, never padded by assuming a width other than 1.
- Names are semantic: PR open / merged / closed / draft; issue open / closed; check passed / failed / pending / skipped; review approved / changes requested / required; run status (ok, failed, running, other); branch, repo, comment, user, label, clock, new-mark, viewed, selected-row, and the four inbox tabs. Colours come with the meaning (green open/ok, purple merged, red closed/failed, yellow pending, grey draft/other), the same in both sets.
- The question: asked only when `icons` is `ask`, the settings file is usable (not read-only), `GH9S_ICONS` is not set, and stdin is a terminal. Otherwise: env set → that set; read-only/broken file or no terminal → `plain` for this run, nothing saved, nothing asked.
- `y` saves `nerd`, `n` saves `plain`, `?` shows how to get a Nerd Font (one sentence and a link), `ctrl+c` quits. After answering, the first screen opens normally. `:icons` asks again (it says so and does nothing when `GH9S_ICONS` is set).
- No test reads or writes the user's real config (use the settings seams of 5a). Icons are process-wide state: every test that switches the set restores `plain` in `t.Cleanup`.
- Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release5b-icons` off `main`; no push/tag without approval. README updated.

## Review Focus

- Alignment: tables with icon cells next to plain cells, CJK titles, truncation in narrow columns (an icon must never be cut off alone or leave a dangling space), the graph boxes and tree lines with icons, the diff file list.
- The question on a 20-column terminal, a resize while it is open, `esc`, unknown keys, holding a key, answering twice quickly, a window-size message arriving first.
- Switching icons in `:settings` while a screen is open: the visible screen changes at once (no stale cached strings: check markdown/diff caches and the inbox table rows, which are built once).
- `GH9S_ICONS=nerd` with a broken settings file; `GH9S_ICONS=plain` with `icons=nerd` in the file (env wins, `:settings` says so).
- Colour: icons carry the state colour; a selected row (reverse video) still shows the icon legibly; no icon is coloured by the wrong state.
- Data races: the active set is read by render code on the UI goroutine and by `tea.Cmd` goroutines that build tables.

---

### Task 1: The icon table

**Files:**
- Create: `internal/icons/icons.go`, `internal/icons/icons_test.go`

**Interfaces:**
- Produces:
  - `type Mode int` with `Plain`, `Nerd`; `func Use(m Mode)`; `func Current() Mode`
  - `type Name int` with the semantic names above (`PROpen`, `PRMerged`, `PRClosed`, `PRDraft`, `IssueOpen`, `IssueClosed`, `CheckPass`, `CheckFail`, `CheckPending`, `CheckSkipped`, `ReviewApproved`, `ReviewChanges`, `ReviewRequired`, `RunOK`, `RunFail`, `RunActive`, `RunOther`, `Branch`, `Repo`, `Comment`, `User`, `Label`, `Clock`, `New`, `Viewed`, `Selected`, `TabAssigned`, `TabReview`, `TabMine`, `TabMentions`)
  - `func Get(n Name) string` (the active set's glyph); `func GetIn(m Mode, n Name) string`
  - `func Parse(s string) (Mode, bool)` ("nerd", "plain")
  - `var Sample []Name` — the glyphs shown by the first-run question

- [ ] **Step 1: Write the failing tests:** both sets define every `Name` (a table-driven check over all names); `plain` equals today's symbols (a pinned list: `✓ ✗ ● ○ ▸ ●` etc.); every `nerd` glyph is exactly one rune, in `U+E000–U+F8FF` (BMP private use), and `ansi.StringWidth == 1`; the nerd glyphs are all different from each other except where a meaning is deliberately shared; `Use` and `Get` race-free (`-race` with parallel readers); `Parse` cases.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/icons` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** (Octicons range from the Nerd Fonts cheat sheet: git-pull-request, git-merge, issue-opened, issue-closed, check, x, dot-fill, skip, comment, git-branch, repo, person, tag, clock, …; the builder lists the code points in a comment next to each).
- [ ] **Step 4: Run** `mise exec -- go test ./internal/icons -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: an icon table with plain and Nerd Font sets`.

### Task 2: Draw from the table

**Files:**
- Modify: `internal/ui/ui.go` (`RunState`, `Symbol`), `internal/graph/graph_render.go`, `internal/tui/tables.go` (run, check and PR-checks labels), `internal/tui/inbox.go` (checks, review, new mark, tab labels), `internal/tui/review.go` (selected, viewed), the PR and issue list builders (a state icon in front of the number cell), `internal/tui/styles.go`
- Test: `internal/tui/icons_view_test.go` plus the existing tests unchanged

**Interfaces:**
- Consumes: Task 1. Produces: no new API; behaviour only. In `plain` mode every output is byte-identical to before.

- [ ] **Step 1: Write the failing tests:** with `icons.Use(icons.Nerd)`: the runs table status cell, a check row, the PR list checks summary, an inbox PR row (checks, review, new mark) and the inbox tab bar, the diff file list (viewed, selected), a graph node, a PR row's state icon (open/draft/merged/closed) and an issue row's, each show the nerd glyph for their meaning with the right colour class and no leftover plain symbol; the same screens in `plain` mode equal the output captured before the change (golden strings in the test, or the untouched old tests); alignment: for every table in nerd mode every row has the same display width per column; a selected row keeps its icon.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui ./internal/ui ./internal/graph -run 'Icons|Nerd'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Status-line messages (`✓ …`, `✗ …`) and dialog markers stay as they are in both modes. Cached renderings that embed icons (markdown/diff caches don't; the inbox rows are built once) are rebuilt when the set changes (`m.rebuildInbox`, etc.).
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: Nerd Font icons on the lists, the graph and the diff`.

### Task 3: The setting, the question, `:icons`

**Files:**
- Create: `internal/tui/iconask.go`, `internal/tui/iconask_test.go`
- Modify: `internal/tui/model.go` (`askIcons bool`), `internal/tui/keys.go` (`:icons`, key routing while the question is open), `internal/tui/view.go`, `internal/tui/completion.go`, `internal/tui/settingsview.go` (apply on change), `README.md`

**Interfaces:**
- Consumes: Tasks 1–2, the settings store (`icons`). Produces: `func (m model) applyIcons() model` (reads the setting and calls `icons.Use`; called at start, after every `Set`/`Cycle` of `icons`, and after the question); the full-screen question `iconAskView(m)` with the sample row built from `icons.Sample`.

- [ ] **Step 1: Write the failing tests:** `Run`-like start with `icons=ask` and a usable store opens on the question, not the runs; `y` saves `nerd` (file read back), applies it, and the next screen shows nerd glyphs; `n` saves `plain`; `?` shows the install sentence and returns to the question; unknown keys do nothing; `ctrl+c` quits; with `GH9S_ICONS=nerd` nothing is asked and nerd is used; with a broken settings file nothing is asked, plain is used, nothing is saved, and the banner says so; `icons=nerd` or `plain` in the file never asks; `:icons` reopens the question (and says so when the env var is set); changing `icons` in `:settings` or with `:set icons nerd` switches the visible screen at once; the question draws within 20×8 and survives a resize; double `y` saves once.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run IconAsk` — Expected: FAIL.
- [ ] **Step 3: Implement** — the "stdin is a terminal" check is a seam (`isTerminal`) so tests can say yes. **Step 4: Run** the suite as in Task 2 — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (ask even when the env var is set; save when read-only; forget to rebuild the inbox rows after a switch) and **commit** `feat: ask about Nerd Font icons on the first start`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (delete `settings.json` and start gh-9s: the question; answer y in a Nerd Font terminal and look at runs, PRs, inbox, diff; `:icons` again; `GH9S_ICONS=plain gh-9s`; a terminal without the font and answer n).
