# Release 3b — readable descriptions and images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PR and issue descriptions/comments render as formatted markdown; images appear as `[image: alt]` markers that open in a full-screen image viewer chosen to suit the terminal.

**Architecture:** A new `internal/markdown` package turns sanitised markdown into styled, width-bounded lines plus a list of image references. `prDetailLines` / `issueLines` / `discussionLines` use it instead of `wrap`. A new `internal/termimg` package detects the terminal and draws an `image.Image` (iTerm2, Sixel, half-blocks). The viewer runs through `tea.Exec` so Bubble Tea releases the terminal while the image is drawn, which keeps graphics sequences out of its line-diff renderer.

**Tech Stack:** Go (mise), goldmark (parser, new dependency; gh-9s writes its own renderer), stdlib `image/{png,jpeg,gif}`, existing lipgloss/ansi.

**Spec:** `docs/superpowers/specs/2026-10-26-gh-9s-release3-design.md` ("Version 2"). Terminal rules and fallbacks there are binding.

## Global Constraints

- All remote text goes through `safe.Text` BEFORE parsing; the markdown renderer emits only its own lipgloss styling. A PR body can never inject an escape sequence.
- Image markers are plain text lines in the detail screens; graphics sequences are written only by the viewer, never inside `View()`.
- Detection reads the environment only (no terminal queries): `GH9S_IMAGES` (`auto|iterm|sixel|blocks|off`), `TERM_PROGRAM=iTerm.app`, `LC_TERMINAL=iTerm2`, `WT_SESSION` → Sixel, else blocks. `$TMUX` set: iTerm2/Sixel only if forced by `GH9S_IMAGES` or `tmux show -gv allow-passthrough` is `on`/`all`, wrapped in tmux's DCS pass-through (ESC doubled); `$STY`/`TERM=screen*` without tmux → blocks. Any doubt degrades to blocks.
- Blocks use truecolor when `COLORTERM` is `truecolor`/`24bit`, else the 256-colour palette.
- Downloads: only the `gh` token, sent only when the image host equals the repo's host; cap 5 MB; decode png/jpeg/gif only; no network in tests (seams).
- Existing keys keep working. Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release3b-markdown` off `main`; no push/tag without approval. README updated.

## Review Focus

- A body with `\x1b]52;…` / CSI / OSC sequences, bidi or control characters renders harmlessly (no byte of it reaches the terminal as an escape).
- Pathological markdown: 5,000 nested list/quote levels, a 1 MB body, an unclosed fence, a table with ragged rows, HTML blocks, CRLF, emoji/CJK widths — no panic, no hang, every line within the width.
- Narrow terminals (width 20) and very wide ones.
- Image: 0-byte, truncated, 50 MB, SVG/WebP (undecodable) → a readable message plus `o` to the browser; never a crash or garbage on screen.
- After the viewer returns, the terminal is back in Bubble Tea's mode and the detail screen is at the same scroll position with the same marker selected.
- Same image reference twice, `<img>` without alt, relative URLs (resolve against the repo's host), `data:` URLs (refuse).
- tmux without passthrough → blocks, never an unterminated escape.

---

### Task 1: Markdown to lines

**Files:**
- Create: `internal/markdown/markdown.go`, `internal/markdown/markdown_test.go`; `go.mod`/`go.sum` (goldmark, pinned)

**Interfaces:**
- Produces:
  - `type Image struct { Line int; Alt, URL string }` (Line = index in `Doc.Lines`)
  - `type Doc struct { Lines []string; Images []Image }`
  - `func Render(src string, width int) Doc` — never panics, every line's display width ≤ `width`.

- [ ] **Step 1: Write the failing tests** (table-driven, assert on text with ANSI stripped via `ansi.Strip`, plus a few assertions that styling exists): `# Title` and `## Sub` bold/coloured, no `#`; `**b**`, `*i*`, `` `c` `` lose their markers; fenced block keeps its lines verbatim with a background style and language tag dropped; `- a`/`1. a`/`- [x] done`/`- [ ] todo` → `• a`, `1. a`, `☑ done`, `☐ todo`; nested lists indent; `> quote` prefixed `│ `; table `|a|bb|\n|-|-|\n|1|2|` aligned in columns, ragged rows tolerated, wider than `width` → cells truncated with `…`; `[text](https://x)` → `text`; `---` → rule of `─`; `<!-- hidden -->` removed; `![shot](https://h/p.png)` and `<img src="https://h/p.png" alt="shot">` → a line `[image: shot]` and `Images=[{Line, "shot", url}]`; missing alt → `[image]`; escape-sequence input (`"hi \x1b[31m\x1b]52;c;AAAA\x07 x"`) → output contains no `\x1b` other than the renderer's own styling (assert by rendering with styling disabled and checking no ESC at all); width 20 and 5 cases; 5,000 nested `>` completes in under a second; empty string → no lines.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/markdown` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `Render`: `safe.Text` → goldmark parse (GFM tables, strikethrough, task lists, raw HTML kept as text except `<img>` and comments) → walk the AST producing styled inline text, then wrap with ANSI-aware width (`ansi.Wrap`/`ansi.StringWidth`). Cap input at 1 MB and nesting depth at 32 (deeper content shown flat).
- [ ] **Step 4: Run** `mise exec -- go test ./internal/markdown -race` — Expected: PASS.
- [ ] **Step 5: Mutation check** (drop `safe.Text` → the escape test fails; revert). **Commit** `feat: render markdown to styled lines`.

### Task 2: Use it in the detail screens, with selectable image markers

**Files:**
- Modify: `internal/tui/prdetail.go` (`prDetailLines`, `discussionLines`, `prDetailBody`, state, keys), `internal/tui/issues.go` (`issueLines`, `issueState`, body, keys)
- Test: `internal/tui/markdown_view_test.go`

**Interfaces:**
- Consumes: `markdown.Render`, `markdown.Image`.
- Produces: `type detailDoc struct { lines []string; images []markdown.Image }`; `prDetailLines`/`issueLines` change to return `detailDoc`; `prDetailState`/`issueState` gain `img int` (selected marker, -1 none); keys `tab` / `shift+tab` move `img` and scroll it into view; the selected marker line is drawn highlighted; `m.openImage(url string) (model, tea.Cmd)` stub declared here, implemented in Task 6 (this task's tests assert `enter` calls it via a seam `showImage`).

- [ ] **Step 1: Write the failing tests:** a PR whose body is `## Plan\n- [x] a\n![shot](https://h/p.png)` shows `Plan`, `☑ a` and `[image: shot]` and no `##`; comments render markdown too (a comment `**ok**` shows `ok` without asterisks); `tab` selects the first marker and scrolls it into view, `tab` again wraps, `shift+tab` goes back; `enter` with a marker selected calls `showImage` with that URL and the tab's repo host; `enter` with none selected does nothing (existing behaviour kept); an issue screen behaves the same; existing scroll tests (`TestDetailScrollsAndClamps`) still pass; no line exceeds the content width.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Markdown|Marker'` — Expected: FAIL.
- [ ] **Step 3: Implement** — replace `wrap(d.Body, …)` / comment wrapping with `markdown.Render`, merge `Images` with line offsets, add selection state and key handling in the screens' `key` funcs.
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Commit** `feat: formatted descriptions and selectable image markers`.

### Task 3: Terminal detection, blocks and iTerm2 drawing

**Files:**
- Create: `internal/termimg/detect.go`, `internal/termimg/draw.go`, tests beside them

**Interfaces:**
- Produces:
  - `type Method int` with `Off, Blocks, ITerm2, Sixel`
  - `type Env struct { Get func(string) string; TmuxPassthrough func() string }`
  - `func Detect(e Env) Method`
  - `func Draw(img image.Image, m Method, cols, rows int, e Env) string` — the complete byte string to write (cursor movement not included); Blocks honour `COLORTERM`; ITerm2 = `ESC ] 1337 ; File=inline=1;width=<cols>;height=<rows>;preserveAspectRatio=1 : <base64 PNG> BEL`; tmux wrapping applied inside `Draw` when `e` says so.

- [ ] **Step 1: Write the failing tests:** `Detect` table (every row of the Global Constraints detection rules, including `GH9S_IMAGES` override of each value, tmux with/without passthrough, `LC_TERMINAL=iTerm2` over ssh, `WT_SESSION`, nothing set → Blocks, `off`); `Draw` Blocks: a 2×2 red/blue image into 2×1 cells gives `▀` with the right truecolor `38;2`/`48;2` codes, and the 256-colour variant uses `38;5;N`; ITerm2 output starts `\x1b]1337;File=` and ends `\a`; tmux-wrapped output is `\x1bPtmux;` + ESC-doubled payload + `\x1b\\`; aspect ratio is kept (a 4:1 image into 10×10 cells uses 10 columns and the fewest rows).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/termimg` — Expected: FAIL.
- [ ] **Step 3: Implement** per the signatures; scaling by nearest-neighbour/box average is the builder's call.
- [ ] **Step 4: Run** `mise exec -- go test ./internal/termimg -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: pick and draw terminal images (blocks, iTerm2)`.

### Task 4: Sixel

**Files:**
- Modify: `internal/termimg/draw.go`; Create: `internal/termimg/sixel.go`, `sixel_test.go`

**Interfaces:**
- Consumes: `Draw`'s `Sixel` branch. Produces: `func encodeSixel(img image.Image, maxColors int) string` (palette ≤ 256, `ESC P q … ESC \`).

- [ ] **Step 1: Write the failing tests:** a 6×6 two-colour image encodes to a string beginning `\x1bPq`, containing `#0;2;` palette entries with percentages (`#0;2;100;0;0`), ending `\x1b\\`; row bands are 6 pixels tall separated by `-`; run-length `!n` is used for repeated sixels; image wider than the cell budget is scaled first; a single-colour image produces one palette entry.
- [ ] **Step 2: Run** — Expected: FAIL. **Step 3: Implement** (median-cut or fixed 6×6×6 palette — builder's call). **Step 4: Run** `mise exec -- go test ./internal/termimg -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: sixel output`.

### Task 5: Fetch an image safely

**Files:**
- Create: `internal/ghcli/image.go`, `internal/ghcli/image_test.go`

**Interfaces:**
- Produces: `func FetchImage(host, rawURL string) (image.Image, error)` with seams `httpDo func(*http.Request) (*http.Response, error)` and `ghToken func(host string) (string, error)` (default `gh auth token --hostname`). Errors are plain sentences (`image is too large (> 5 MB)`, `unsupported image type`, `download refused (HTTP 404)`).

- [ ] **Step 1: Write the failing tests** (fake `httpDo`): token sent when the URL host equals `host`, not sent to another host, not forwarded on a redirect to another host; relative or schemeless URL resolved against `https://<host>`; `data:` and `file:` URLs refused; body over 5 MB rejected without reading it all (limit reader); png/jpeg/gif decode; WebP/garbage → `unsupported image type`; 404/403 → `download refused (HTTP n)`; empty body → error.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/ghcli -run Image` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** `mise exec -- go test ./internal/ghcli -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: download images with the gh token, safely`.

### Task 6: The viewer

**Files:**
- Create: `internal/tui/imageview.go`, `internal/tui/imageview_test.go`
- Modify: `internal/tui/prdetail.go`, `internal/tui/issues.go` (the `o` key on a selected marker opens its URL), `README.md`

**Interfaces:**
- Consumes: Task 2's `showImage` seam / `openImage`, `ghcli.FetchImage`, `termimg.Detect/Draw`.
- Produces: `imageViewer` implementing `tea.ExecCommand` (`Run`, `SetStdin/SetStdout/SetStderr`) — on `Run`: fetch (message `downloading…` first), clear the screen, draw the image sized to the terminal minus one line, print `esc/q back · o open in browser` (or the error and the same hint), read one key in raw mode (`golang.org/x/term`, already in the module graph), `o` opens the browser through the `openURL` seam, anything of `esc`/`q`/`enter` returns; the terminal state is restored even on error or panic (`defer`). A `tea.Exec` callback returns `imageDoneMsg{err}`; the model then shows `m.status` for errors and leaves scroll and marker unchanged. `GH9S_IMAGES=off` skips the viewer and opens the browser directly.

- [ ] **Step 1: Write the failing tests** (viewer logic with an injectable `in io.Reader`/`out io.Writer`, fake `FetchImage`): success path writes the `Draw` output and the hint, returns on `esc`; `o` calls `openURL` once then keeps waiting; fetch error prints the message and the hint and still waits for a key; `off` opens the browser with no drawing; the model after `imageDoneMsg` keeps `scroll` and `img`; a failed return sets status.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Image` — Expected: FAIL.
- [ ] **Step 3: Implement** `imageViewer`, `openImage`, the `imageDoneMsg` handling, the `o`-on-marker behaviour, and a README section (markers, `tab`, `enter`, `GH9S_IMAGES`, tmux note).
- [ ] **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=1'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (viewer skips terminal restore → test fails; revert). **Commit** `feat: full-screen image viewer`.
- [ ] **Step 6: Manual check by the user** (I cannot see their terminals): list in the final message exactly what to try in iTerm2, Windows Terminal and Terminal.app.
