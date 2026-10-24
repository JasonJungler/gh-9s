# Release 5a — the settings system and `:settings` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One settings registry, one `settings.json`, a `:settings` screen and a `:set key value` command, with the merge dialog's remembered choice moved onto it; the registry is the place a later release adds a setting.

**Architecture:** A new package `internal/settings` owns the registry, the values with their sources, loading/validating/saving and the environment overrides; it knows nothing about the TUI. `internal/tui` holds one `*settings.Store` in the model, a `viewSettings` screen on the screen registry, and the `:set` command. The icons, preview and layout settings the later parts need are registered here with their defaults (nothing reads them yet except the merge dialog).

**Tech Stack:** Go (mise), existing screen registry and dialog code.

**Spec:** `docs/superpowers/specs/2026-10-27-gh-9s-release5-design.md` ("Part 1").

## Global Constraints

- Settings registered now: `icons` (enum `ask|nerd|plain`, default `ask`, env `GH9S_ICONS` accepting `nerd|plain`), `preview` (bool, default true), `preview_width` (int 25–60, default 40), `preview_min_columns` (int 80–400, default 120), `delete_branch_after_merge` (bool, default false). Every setting declares key, label, description, kind, default, and optional env name.
- Each value carries its source: default, file, or env. The environment wins; a change made while an env var overrides is saved to the file and reported as having no effect for now.
- The file is `settings.json` in the gh-9s config directory (path override `GH9S_SETTINGS`); written atomically (temp file in the same directory, then rename), permissions 0600 in a 0700 directory; unknown keys are preserved on save; invalid JSON is an error that leaves the file untouched and makes the store read-only for the run (every save returns the same sentence); an invalid value for a known key falls back to the default in memory, is reported, and is not rewritten until the user changes that key.
- An existing `prefs.json` is read once: `delete_branch_after_merge` is carried into the settings (only if `settings.json` does not already have it) and `prefs.json` is left alone.
- `:settings` and `:set` must work on a read-only store (they show the problem; changes say "not saved: <reason>" and still apply for the session).
- No test reads or writes the user's real config: every path comes from a seam (`settingsPath`), and `GH9S_*` variables are set with `t.Setenv`.
- Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release5a-settings` off `main` (after the design branch is merged); no push/tag without approval. README updated.

## Review Focus

- A settings file edited by hand to nonsense: wrong types (`"preview": "yes"`), out-of-range numbers, huge numbers, a string with escape sequences or newlines in a value that is displayed, a 10 MB file, a directory where the file should be, a read-only directory, a symlink.
- Two gh-9s instances saving different settings at once (the second must not erase the first's key: re-read before writing, as `internal/drafts` does).
- Env var set to garbage (`GH9S_ICONS=banana`): ignored with a note, not an error screen.
- `:set` with unknown keys, extra words, `icons ask` (allowed? yes: it re-arms the question), numbers with signs and spaces, `preview toggle`, case.
- The merge dialog: first merge asks with the saved default, a broken settings file does not break merging (the choice still applies for that merge), and nothing is written to `prefs.json` any more.
- `:settings` on a 40-column terminal, with 100 settings (scrolling), after a resize.

---

### Task 1: The registry and the store

**Files:**
- Create: `internal/settings/settings.go`, `internal/settings/registry.go`, `internal/settings/settings_test.go`

**Interfaces:**
- Produces:
  - `type Kind int` (`Bool`, `Enum`, `Int`); `type Def struct { Key, Label, Desc string; Kind Kind; Default any; Options []string; Min, Max int; Env string }`
  - `var Registry []Def` (the five settings above, in display order) and `func Lookup(key string) (Def, bool)`
  - `type Source int` (`FromDefault`, `FromFile`, `FromEnv`)
  - `type Store struct{ … }`; `func Load(path string, getenv func(string) string, legacyPrefs string) (*Store, error)` — never returns nil: on a broken file the store is read-only and the error says why
  - `func (s *Store) Bool(key string) bool`, `Int(key string) int`, `String(key string) string`, `Source(key string) Source`, `Notes() []string` (problems found while loading: invalid values, bad env), `ReadOnly() error`
  - `func (s *Store) Set(key, value string) (applied string, err error)` — parses/validates against the Def (bool accepts on/off/true/false/yes/no/1/0 in any case; enums case-insensitively; ints trimmed, range-checked), applies in memory, saves unless read-only; returns the normalised value; `err` for an invalid value (nothing changes) and a distinguishable `ErrNotSaved` wrapping the reason when the value was applied but not saved
  - `func (s *Store) Cycle(key string, dir int) (string, error)` — next/previous option, toggle for bool, ±1 step for int (±5 for ranges wider than 50)

- [ ] **Step 1: Write the failing tests:** defaults with no file; file values with a legacy `prefs.json` carried over only when the key is absent; env wins and `Source` says so; `GH9S_ICONS=banana` → default plus a note; wrong types and out-of-range values in the file → default plus a note, other keys unaffected; unknown keys survive a save (read the file back and compare); invalid JSON → read-only store, file bytes untouched after `Set`, `Set` still applies in memory and returns `ErrNotSaved`; `Set` validations table (bools, enums, ints, bad keys, bad values, case, whitespace); `Cycle` for each kind including wrap-around; a second `Store` on the same file saving a different key does not erase the first (re-read before write); permissions 0600/0700; 10 MB file refused as "too large"; directory-in-place-of-file and unwritable directory give `ErrNotSaved`.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/settings` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/settings -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: a settings registry and store`.

### Task 2: The model holds settings; the merge choice moves onto them

**Files:**
- Modify: `internal/tui/model.go` (`settings *settings.Store`, banner on a broken file), `internal/tui/merge.go`, `internal/tui/prefs.go` (reduced to the one-time read path or removed), tests that used `prefsFile`
- Test: `internal/tui/merge_test.go`, `internal/tui/settings_test.go`

**Interfaces:**
- Consumes: Task 1. Produces: `model.settings`; `func settingsPath() string` seam in `tui`; `Run()` loads with `os.Getenv`, the legacy `prefs.json` path, and puts `store.ReadOnly()` and `store.Notes()` on the banner.

- [ ] **Step 1: Write the failing tests:** the merge dialog starts with the saved `delete_branch_after_merge`, saves the new answer to `settings.json` (not `prefs.json`), and with a broken settings file still merges, shows "could not be saved" and writes nothing; an old `prefs.json` value shows up as the default of the toggle once; the banner mentions a broken file and each note once.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Merge|Settings'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: the merge choice lives in the settings`.

### Task 3: The `:settings` screen and `:set`

**Files:**
- Create: `internal/tui/settingsview.go`, `internal/tui/settingsview_test.go`
- Modify: `internal/tui/model.go` (`viewSettings`), `internal/tui/keys.go` (`:settings`, `:set`), `internal/tui/completion.go` (commands, key and value completion), `internal/tui/view.go` (hint line), `README.md`

**Interfaces:**
- Consumes: Tasks 1–2. Produces: `viewSettings` registered with `register`; rows of label, value, source tag (`file`, `env`, blank for default), description under the selected row; `enter`/`space`/`→` cycle forward, `←` back, `esc` leaves; a status line `✓ preview = off` or `not saved: <reason>` or `set by GH9S_ICONS: no effect for now`; `:set key value` and `:set key` (shows the value); completion of `set ` with the keys and of `set icons ` with the options.

- [ ] **Step 1: Write the failing tests:** `:settings` lists all five settings with their current values and sources; cycling `icons` goes ask → nerd → plain → ask and saves each time (file read back); a bool toggles; an int steps within its range and stops at the ends; an env-overridden setting shows `env` and says the change has no effect; a broken file shows the reason at the top and changes apply but say "not saved"; `:set preview off` applies, `:set preview maybe` says what is allowed and changes nothing, `:set nope 1` says unknown setting; `:set` completion offers keys then values; a 40-column terminal and a 100-setting registry (test-only registry swap) scroll and truncate without breaking the layout; `esc` returns to where the user came from.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Settings` — Expected: FAIL.
- [ ] **Step 3: Implement**; document `:settings`, `:set`, the file, the env overrides and the settings in the README. **Step 4: Run** the suite as in Task 2 — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (skip the re-read before write; let the env win silently; save on a read-only store) and **commit** `feat: the :settings screen and :set`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (open `:settings`, change values, look at `settings.json`, break the file by hand and reopen, set `GH9S_ICONS=nerd` and look at the source tag).
