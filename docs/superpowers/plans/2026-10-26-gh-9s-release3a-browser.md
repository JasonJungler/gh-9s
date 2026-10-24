# Release 3a — open links in the right browser (WSL) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `o` / Enter-on-a-non-Actions-check open the URL in the Windows browser under WSL, honour `$BROWSER`, and show a status message when opening fails instead of dropping the error.

**Architecture:** A pure function in `internal/ghcli` decides which command opens a URL from (GOOS, `$BROWSER`, WSL?, available tools); `OpenURL` runs that command. The four `_ = openURL(...)` call sites in `internal/tui` go through one helper that puts a failure in `m.status`.

**Tech Stack:** Go (mise), existing seams (`openURL` package var in `tui`).

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release3-design.md` ("Version 1 — browser opening on WSL")

## Global Constraints

- Only `internal/ghcli` runs external programs; tests never launch a browser or touch the network.
- Order of choice (spec): `$BROWSER` if set; on WSL `wslview`, else `cmd.exe /c start "" <url>`; otherwise `open` (darwin), `rundll32 url.dll,FileProtocolHandler` (windows), `xdg-open` (other).
- WSL = `WSL_DISTRO_NAME` non-empty, or `/proc/version` contains `microsoft` (case-insensitive).
- Remote-derived URLs are passed as a single argument, never through a shell string.
- Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release3a-browser` off `main`; no push or tag without the user's approval.

## Review Focus

- A URL containing `&` or `%` reaches `cmd.exe` intact (`start "" <url>`: the empty title argument matters, and `&` must not be read as a command separator — pass the URL as its own argv element, and prefer `wslview` when present).
- `$BROWSER` holding several alternatives (`a:b`), a `%s` placeholder, or extra arguments (`firefox --new-tab`) works; an empty or whitespace-only value is treated as unset.
- A browser launcher that fails to start (missing binary) returns an error that the user sees, rather than silence.
- Spawned launchers are reaped (no zombie accumulating per keypress).
- Non-WSL Linux and macOS behaviour is unchanged.

---

### Task 1: Decide the command (pure)

**Files:**
- Create: `internal/ghcli/browser.go`
- Test: `internal/ghcli/browser_test.go`

**Interfaces:**
- Produces:
  - `func IsWSL(getenv func(string) string, readFile func(string) ([]byte, error)) bool`
  - `type BrowserEnv struct { GOOS string; Browser string; WSL bool; Has func(name string) bool }`
  - `func BrowserCommand(e BrowserEnv, url string) (name string, args []string)`

- [ ] **Step 1: Write the failing tests** in `browser_test.go`:
  - `TestIsWSL`: `WSL_DISTRO_NAME=Ubuntu` → true; no env but `/proc/version` = "Linux version 5.15.90.1-microsoft-standard-WSL2" → true; "…-Microsoft" (uppercase, WSL1) → true; plain Linux version string → false; unreadable file and no env → false.
  - `TestBrowserCommand` table, each case a `BrowserEnv` with `Has` backed by a set, url `https://github.com/a/b/pull/1?x=1&y=2`:
    - darwin → `open`, `[url]`; windows → `rundll32`, `["url.dll,FileProtocolHandler", url]`; linux → `xdg-open`, `[url]`.
    - WSL with `wslview` available → `wslview`, `[url]`.
    - WSL without `wslview` → `cmd.exe`, `["/c", "start", "", url]`.
    - `Browser="firefox --new-tab"` → `firefox`, `["--new-tab", url]` (also on WSL: `$BROWSER` wins).
    - `Browser="a:b"` → uses the first alternative whose command `Has` returns true (`a` missing, `b` present → `b`); none present → first alternative.
    - `Browser="mybrowser %s --private"` → `mybrowser`, `[url, "--private"]` (`%s` replaced by the url as one argument, not appended again).
    - `Browser="   "` → treated as unset (falls through to GOOS/WSL rules).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run 'IsWSL|BrowserCommand'` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** the three declarations in `browser.go`. `$BROWSER` is split on `:` into alternatives, each split on whitespace; first word is the command. Never invoke a shell.
- [ ] **Step 4: Run the same command** — Expected: PASS.
- [ ] **Step 5: Commit** `feat: decide which command opens a URL (WSL, $BROWSER)`.

### Task 2: Run it from OpenURL

**Files:**
- Modify: `internal/ghcli/gh.go:65-75` (`OpenURL`); remove the `runtime` import if unused
- Test: `internal/ghcli/browser_test.go`

**Interfaces:**
- Consumes: Task 1's `IsWSL`, `BrowserEnv`, `BrowserCommand`.
- Produces: `func OpenURL(url string) error` (same signature); package vars `startCommand = func(name string, args ...string) error` (starts the process and reaps it in a goroutine; returns the start error) and `lookPath = exec.LookPath`, so tests can intercept.

- [ ] **Step 1: Write the failing tests:** `TestOpenURLStartsTheChosenCommand` (replace `startCommand` with a recorder; `$BROWSER` set via `t.Setenv` to `fakebrowser`; expects name `fakebrowser`, args `[url]`); `TestOpenURLReturnsTheStartError` (recorder returns an error → `OpenURL` returns it, wrapped with the command name so the message reads `fakebrowser: <err>`).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run OpenURL` — Expected: FAIL.
- [ ] **Step 3: Implement** `OpenURL`: build `BrowserEnv` from `runtime.GOOS`, `os.Getenv("BROWSER")`, `IsWSL(os.Getenv, os.ReadFile)`, `Has` via `lookPath`; call `BrowserCommand`; call `startCommand`. `startCommand` default uses `exec.Command(...).Start()` then `go cmd.Wait()`.
- [ ] **Step 4: Run** `mise exec -- go test ./internal/ghcli` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: OpenURL uses the decided command and reports start errors`.

### Task 3: Show a failure instead of dropping it

**Files:**
- Modify: `internal/tui/prdetail.go:80`, `internal/tui/issues.go:234`, `internal/tui/checksjump.go:27`, `internal/tui/diffview.go:202`; add `func (m model) open(url string) model` next to the `openURL` seam (`prdetail.go:18` area, or `cmds.go` if that fits the file better)
- Modify: `README.md` (one paragraph under the key table: how links open, WSL, `$BROWSER`)
- Test: `internal/tui/browser_test.go`

**Interfaces:**
- Consumes: package var `openURL func(string) error`.
- Produces: `func (m model) open(url string) model` — calls `openURL`; on error sets `m.status` to `can't open the browser: <err>` and returns the model; on success returns it unchanged.

- [ ] **Step 1: Write the failing tests:** with `openURL` replaced by a fake that returns an error, pressing `o` in the PR detail, the issue detail and the diff view, and Enter on a non-Actions check, each leaves `m.status` containing `can't open the browser`; with a fake that succeeds, status stays empty and the URL was passed exactly once (reuse the existing `recordOpenURL` helper in `checksjump_test.go` for the success case).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Browser` — Expected: FAIL.
- [ ] **Step 3: Implement** `open` and switch the four call sites to `m = m.open(url)` (returning the updated model in each key handler).
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: no gofmt output, all `ok`.
- [ ] **Step 5: Mutation check** — make `open` ignore the error; the new tests must fail; revert.
- [ ] **Step 6: Commit** `feat: show why a link could not be opened` (include the README paragraph).
