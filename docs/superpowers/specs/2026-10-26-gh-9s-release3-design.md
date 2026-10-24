# gh-9s release 3 — design

Three independent changes, built in this order, one version each: (1) open links in the right browser on WSL, (2) readable PR/issue descriptions with images, (3) a code-review screen with side-by-side diff.

## Why

The user reviews other people's PRs in a terminal and finds it worse than the web: the description is raw markdown, the diff cannot be read side by side, and on WSL a link opens (or fails to open) the wrong browser. Success is that a review can be done start to finish in gh-9s without needing the web page for reading.

User's terminals: iTerm2 and macOS Terminal.app (macOS), Windows Terminal (WSL). Shells are bash and zsh.

## Non-goals / anti-goals

- No inline-review-comment *editing* of other people's comments, no suggested-changes blocks, no multi-line comment ranges (single line only).
- No image support that corrupts the screen: if the terminal cannot show images safely, never emit graphics sequences.
- Remote text must stay sanitised (`safe.Text`): rendering markdown must not let a PR author inject escape sequences. Only gh-9s's own styling and image sequences may reach the terminal.
- No new network access outside `gh` (images are fetched with the `gh` auth token, via `internal/ghcli`; tests use seams and never touch the network).

## Version 1 — browser opening on WSL

- Detect WSL: `WSL_DISTRO_NAME` set, or `/proc/version` contains `microsoft` (case-insensitive).
- Opening a URL, in order: `$BROWSER` if set (it may contain args; run it with the URL); on WSL `wslview`, else `cmd.exe /c start "" <url>` (URL must be passed so `&` in query strings survives); otherwise the existing per-OS behaviour (`open`, `rundll32`, `xdg-open`).
- Errors are shown in the status line instead of being dropped (today `openURL` errors are ignored).
- Lives in `internal/ghcli.OpenURL`, with the detection and command choice split into a pure function so it is unit-tested without launching anything.

## Version 2 — readable descriptions and images

Applies to PR description/comments/reviews and issue description/comments.

- Render markdown with styling: headings (bold, coloured), bold/italic/inline code, fenced code blocks with a background, bullet/numbered/task lists (`[x]` / `[ ]` shown as checkboxes), block quotes, tables aligned to column widths, links shown as their text with the URL kept available (`o` still opens the page), horizontal rules, HTML comments hidden. Wrapped to the screen width. Unknown constructs degrade to plain text, never panic.
- Images (`![alt](url)` and `<img src>`): in the description they appear as a one-line marker `[image: alt]` (decided by the user: no graphics inline, so scrolling and Bubble Tea redraws can never leave remnants). With the cursor on a marker (`tab`/`shift+tab` move between markers), `enter` opens a full-screen image view; `esc` returns to the same scroll position; `o` opens the image URL in the browser.
- The full-screen view draws the image with the best method the terminal supports, chosen automatically: iTerm2 inline-image protocol, Sixel, else half-block (`▀`) colour approximation (truecolor when `COLORTERM` says so, otherwise 256-colour, so macOS Terminal.app gets a usable picture), else a message with the URL. `GH9S_IMAGES=auto|iterm|sixel|blocks|off` overrides (env var only, no config file); `off` means markers only and `enter` opens the browser.
- Detection must not query the terminal (no reading replies while Bubble Tea owns the input). Use environment only: `GH9S_IMAGES`, then `TERM_PROGRAM=iTerm.app` or `LC_TERMINAL=iTerm2` (ssh forwards the latter), `WT_SESSION` (Windows Terminal, Sixel), else blocks. Blocks are plain text, so they are the safe default in tmux, screen, ssh and VS Code's terminal. Inside tmux (`$TMUX` set) iTerm2 and Sixel sequences are used only if the user forced them with `GH9S_IMAGES` or `tmux show -gv allow-passthrough` reports `on`/`all`, and then wrapped in tmux's DCS pass-through (ESC doubled); otherwise blocks. In screen: blocks. A wrong guess must degrade to blocks, never to garbage.
- Images are downloaded only when the view opens, with the `gh` token, cached on disk for the session, rejected over about 5 MB, scaled to fit the screen. If a download is refused (typical for private-repo attachment links) the view says so and offers `o`; there is no extra API call to obtain signed links. Tests use seams, never the network.
- The user's terminals: iTerm2, Windows Terminal (WSL), macOS Terminal.app; used inside tmux and over ssh as well.
- Builder's calls: choice of markdown parser/library (prefer a small, maintained Go dependency such as goldmark for parsing with gh-9s's own renderer, over glamour's whole-document styling, since width, sanitising and image placement need control); how Sixel is encoded.

## Version 3 — review screen

Entered from the PR detail (replaces the current single-column diff screen's role; `d` still opens it).

- **Layout:** left pane is the file list (name, +/- counts, a mark for files viewed, a mark for files with comments). Right pane is the diff of the selected file. `tab` switches the active pane. Both panes scroll independently.
- **Side-by-side diff:** old on the left, new on the right, aligned line numbers, scrolled together; additions/deletions coloured; within a changed line pair the changed *words* are highlighted (word-level diff of the paired old/new lines). Falls back to the existing unified single-column view when the width is too small (less than about 120 columns), and `s` toggles side-by-side/unified by hand. Long lines wrap or are cut with a visible marker (builder's call; consistent in both panes).
- **Navigation:** `j/k` lines, `n/N` next/previous file, `]`/`[` next/previous hunk, `g/G`, `/` search in the diff, `v` toggle "viewed" for the file (kept only for this session, per PR head commit — a new head commit clears it).
- **Existing review threads** (other people's inline comments) are shown under the line they refer to, marked and indented; outdated threads are shown dimmed with a note.
- **Inline comments:** `c` on a line (new side, or old side when the line was deleted) opens a text dialog; the comment is queued, shown in place with a "pending" mark, and listed in a count in the footer. Queued comments are submitted together with the review verdict (approve / request changes / comment) as one GitHub review, through `gh api` (the REST/GraphQL review endpoint with `comments`, pinned to the head commit the user was looking at). `x` on a pending comment deletes it. Leaving the screen with pending comments asks for confirmation; they are not kept after quitting.
- **Submitting:** the existing review flow (verdict choice, body, confirmation naming `owner/repo#N`) is reused; the confirmation also states how many inline comments go with it. The review is sent against the head commit shown; if the PR head moved since the diff was fetched, submission is refused with a message and the diff is refetched (the user's pending comments are kept for the refetched diff only if their file and line still exist; otherwise they are listed as dropped).
- **Safety constraints carried over:** actions use the repo/number the screen was opened for, not `m.base`; stale async results are dropped by identity; remote text sanitised; body passed as a single `--field` argument, never shell-built.

## Calls I made (user may overrule)

- A pending (unsubmitted) review the user already has on the PR makes submission fail on GitHub's side; show GitHub's reason and say to finish or discard it on the web. No automatic merge or discard.
- The review screen shows the whole diff: today's line cap is removed in favour of a very high safety limit; past it the file is marked "truncated" and its remainder cannot be commented on.
- Replying to or resolving other people's threads is out of scope (shown only).

## Constraints

- Go, Bubble Tea v1, lipgloss, `gh` for all GitHub access; all outside calls (gh, browser, image fetch, `/proc`) behind package-level seams; tests never touch the network and TestMain keeps a stub `gh` first on PATH.
- No release without the user's approval; each version is its own branch, TDD, a fresh-reviewer pass, and the finishing menu.
- README updated with each version.

## Left to the builder

Markdown library choice, Sixel encoder, image decoding/scaling library, exact colours, exact narrow-width threshold, how long lines are handled in side-by-side, cache layout for images, key conflicts resolution within the existing key map (keep existing keys working).
