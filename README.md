# gh-9s

A [k9s](https://k9scli.io)-style terminal UI for GitHub, as a `gh` extension.

- **Actions:** trigger workflows, watch runs as a live job graph, keep favorite workflows one keypress away.
- **Pull requests:** list and filter, see checks (and jump into the Actions run behind a check), read the diff, comment, review and merge.
- **Issues:** list and filter, read, comment, close and reopen, manage labels and assignees.
- **Repos:** an interactive picker to move between any repo you can reach.

## Install / develop

```sh
mise install                          # pins go + gh (see mise.toml)
mise exec -- go build -o gh-9s .      # the binary must be named after the repo: gh-9s
gh extension install .                # then run: gh 9s
```

Run it inside any git repo with a GitHub remote. All GitHub access goes through the `gh` CLI, so
authentication, proxies and GitHub Enterprise follow whatever `gh` is logged in to (or `GH_HOST`).
Enterprise has not been tested yet.

Pushing a `v*` tag publishes precompiled binaries via `.github/workflows/release.yml`.

## Views

| key / command | view |
|---|---|
| `1` / `:runs` | workflow runs (refreshes every 5s) |
| `2` / `:wf` | workflows; your favorites are pinned on top with a ★ |
| `3` / `:pr` | open pull requests (`*` marks the current branch); `enter` opens one |
| `4` / `:issues` | issues; `enter` opens one |
| `P` / `:cpr` | checks of the current branch's PR |
| `:fav` | favorites in three tabs (`tab`/`shift+tab`/`h`/`l`): **Workflows** from every repo with the latest run's status, pinned **Repos** (`enter` switches to one), pinned **Projects** |
| `:fav repo [OWNER/NAME]` | pin the current (or the given) repo |
| `:<alias>` | open the runs of the favorite with that alias |
| `:repo` | open the repo picker (below) |
| `:repo OWNER/REPO` | switch every view to another repo (`HOST/OWNER/REPO` for another host). The repo is checked first. The current-branch PR (`P`, `:cpr`) only exists in the current directory's repo. |

## Keys

Everywhere: `j/k` move, `g/G` top/bottom, `/` filter, `:` command bar, `esc` back, `R` refresh, `o` open in browser, `q` quit.

Links (`o`) open in your browser: `$BROWSER` if set, otherwise the system default. Under WSL they open in the Windows browser (`wslview` if installed, else `cmd.exe /c start`). If it can't be opened, the reason is shown in the status line.

PR and issue descriptions and comments are rendered as formatted markdown (headings, lists, task lists, tables, code blocks, quotes). Images show as `[image: alt]` markers: `tab` / `shift+tab` select one, `enter` opens it full screen (`esc` or `q` to come back, `o` to open it in the browser). The picture is drawn with iTerm2's inline images in iTerm2, Sixel in Windows Terminal, and coloured half-block characters everywhere else (macOS Terminal, VS Code, ssh). Inside tmux, graphics are used only when tmux's `allow-passthrough` is on; otherwise you get the half-block picture. `GH9S_IMAGES=auto|iterm|sixel|blocks|off` overrides the choice (`off` sends images to the browser). Images are fetched with your `gh` login (only from your GitHub host), up to 5 MB, as png, jpeg or gif.

**Reviewing a PR** (`d` on a PR): on a terminal at least 120 columns wide the diff is shown side by side — old on the left, new on the right, changed words highlighted — with the file list on the left. The file list is a directory tree: directories first, a chain of single-child directories on one row (`internal/tui`), each directory showing its totals; in the list `enter`/`space` fold a directory, `h`/`l` fold and unfold (`h` on a file selects its directory), and `v` on a directory marks every file under it viewed. With Nerd Font icons (`:icons`) folders and files get an icon by language and kind (Go, TypeScript, Markdown, YAML, Dockerfile, lock files…). Narrower terminals get the single-column diff; `s` switches by hand. `tab` moves between the file list and the diff, `]` / `[` jump between files, `}` / `{` between hunks, `v` marks a file viewed (kept until the PR gets a new commit), `/` searches. Comments other people left on lines of the diff appear under those lines (`◆n` in the list counts them).

Fenced code blocks in descriptions and comments (```` ```go ````) are coloured the same way, by the language written after the fence; blocks without one, or with an unknown one, stay plain.

**Settings** (`:settings`, or `:set key value` from the command bar, which completes keys and values): a screen listing every setting with its value and where the value comes from (`file`, `env`, or blank for the default); `j/k` move, `enter` / `→` change to the next value, `←` to the previous, and every change is saved at once to `settings.json` in the gh-9s config directory (set `GH9S_SETTINGS` to use another file). Environment variables override the file (`GH9S_ICONS=nerd|plain`) and the screen says when a change has no effect because of one. Unknown keys in the file are kept; a file that is not valid JSON is never overwritten (gh-9s starts with the defaults, says so, and changes last only for the session). The old `prefs.json` choice (delete branch after merge) is carried over once.

**Mouse wheel:** the wheel scrolls any screen, three lines per notch (over the preview pane it scrolls the pane); clicks are not used. While the mouse is on, the terminal's own text selection needs a modifier: hold Option in iTerm2 and Terminal.app, Shift in Windows Terminal, tmux and most Linux terminals. Turn it off with the `mouse` setting (`:set mouse off`), which takes effect at once.

**Help:** `?` on any screen lists every key of that screen (the line at the bottom has room only for the common ones), then the keys every screen has; `j`/`k` scroll it and `esc` or `?` close it.

**Detail screens:** on a terminal about 100 columns of text wide or more, the PR screen (`enter` on a PR) shows the description and discussion on the left and the sidebar (reviewers with status, assignees, labels, projects, milestone, closed issues) in a column on the right, as on GitHub; the issue screen does the same with labels and assignees. Narrower terminals keep one column.

**Preview pane:** on the pull request list, the issue list and every tab of the inbox, a terminal at least 120 columns wide gets a pane on the right (like gh-dash) that shows the selected item: for a PR its title, author, `base ← head`, labels, review decision, check counts with the names of the failing ones, size and description (rendered as markdown) and the latest comments; for an issue its labels, assignees, description and latest comments. The detail loads after the selection has rested for a moment (so holding `j` does not fire a request per row), is kept for five minutes (50 items) and is fetched again when the item changed. When the pane is wide enough (about 72 columns of text: a terminal of 190+ columns at the default width, or less with a larger `preview_width`) a PR's sidebar — reviewers with their status, assignees, labels, projects, milestone and the issues it closes — sits in a column to the right of the description and comments, as on GitHub; in a narrower pane it comes above the description. `J` / `K` scroll the pane, `p` hides or shows it for the session, `enter` still opens the full screen. The settings `preview` (on/off), `preview_width` (percent, 25–60) and `preview_min_columns` (80–400) are in `:settings`. Narrower terminals get the full-width lists as before.

**Slow checks column:** on a big repository GitHub can take many seconds to compute the checks of 50 PRs (it used to end in a 504). The PR list now loads without them, the CHECKS column shows `…` and fills in when a second request answers (the newest 20 if it must ask for less); if that fails the column stays `-` and the status line says why.

**Icons:** the first time gh-9s starts it shows a row of icons and asks whether you can see them (not boxes or question marks). Answer `y` if your terminal uses a [Nerd Font](https://www.nerdfonts.com) and the lists, checks, runs, graph, inbox and diff get GitHub-style icons for PRs, issues, checks and reviews; answer `n` to keep the plain symbols. The answer is saved as the `icons` setting; change it in `:settings` (or `:set icons nerd|plain|ask`), ask again with `:icons`, or force it for one run with `GH9S_ICONS=nerd|plain`. Nothing is asked when the settings file cannot be used or there is no terminal; the plain symbols are used then.

**The inbox** (`0` or `:inbox`): what needs you, across all repos and every host `gh` is logged in to — tabs for *assigned issues* (opens on this), *review requested*, *my PRs* (with checks and review state) and *mentions*; switch with `tab` / `shift+tab` or `h` / `l`. `enter` opens an item in the PR or issue screens (the review screen and all actions included), `esc` comes back to the same tab and row, `o` opens it in the browser, `/` filters what is loaded. It refreshes itself about once a minute (`R` forces it, at most every few seconds); rows that appeared since the last refresh are marked `●` until you leave the tab. A host that cannot be reached is shown as a note while the others still list.

**Commenting while you review:** a cursor (`▶`) walks the diff with `j/k`. `c` writes an inline comment on the cursor's line (the new side; `C` for the old side of a changed or context line). `V` starts a range: move to the last line, then `c` (`esc` cancels). Comments are *drafts*, shown under their line as "you (draft)" and kept on disk in `drafts.json` next to `favorites.json`, per pull request, so closing gh-9s loses nothing; `x` removes the draft on the cursor's line, `X` discards them all. Each draft remembers the commit it was written for; after a push, drafts from the older commit are marked and `A` keeps them for the new one (the lines must still exist). `r` sends them: choose approve, request changes or comment (the last two need a message), confirm, and everything goes out as one GitHub review pinned to the commit you were looking at. If the PR has new commits since, nothing is sent (press `R` to reload; drafts whose lines are gone are reported and dropped).

Code in diffs is coloured by language (chroma, chosen by file name), in both the side-by-side and the single-column view. It is computed per hunk, so a string or comment that starts above a hunk can be coloured wrongly; unknown languages, lines over 400 characters, and anything past a 2 MB / 3-second budget stay in plain red/green.

The `:` command bar, `/` filter, log search and alias entry open in a box under the header, like k9s. The command bar completes as you type: the rest of the best match shows in grey, `Tab` (or `→`) accepts it and `Tab` again cycles through the other matches. Commands complete from the built-ins, your favorite aliases and (after `:repo `) your recent and cached repos. `Enter` on a unique prefix runs it (`:wo` opens workflows), and `↑`/`↓` walk the commands you typed before. Dialogs, forms and the repo picker are boxed at the top too.

**Runs** — `enter` job graph · `l` full log in pager · `L` failed log · `w` watch (`gh run watch`) · `r` rerun · `F` rerun failed · `c` cancel · `d` delete (each asks first, naming the repo and run, and says why when it does not apply; the same keys work on the job graph and the job list, and an accepted re-run goes to that run's graph and is watched again) · on the job graph `f` opens the log of the first failed job at its first error

**Workflows / favorites** — `enter` runs · `t` trigger form · `f` favorite/unfavorite · `a` set alias · `e` enable/disable

**Favorites tabs (`:fav`)** — `tab`/`l` next tab · `shift+tab`/`h` previous · Repos: `enter` switch to the repo · `a` pin · `f`/`d` unpin · `o` open. Projects: `enter` open the board · `a` pin (pick from the list of your and your organisations' Projects, or type `[HOST/]OWNER/NUMBER`) · `f`/`d` unpin · `o` open. In the `:repo` picker pinned repos carry a ★ and `ctrl+f` pins or unpins the selected one. `f` pins or unpins wherever the thing is on screen: the repo from the runs, pulls and issues lists, the Project from its board (`a` is only for typing a name to pin).

**Job graph** (watching a run)
- `hjkl` or arrows move between jobs, `enter` opens the job's log, `v` switches style, `ctrl-d/u` scroll
- Styles: **boxes** (default, left-to-right with arrows), **tree** (top-to-bottom, fits narrow terminals), **lanes** (timeline). Boxes too wide for the terminal fall back to the tree.
- In the boxes style, lines connect only neighbouring stages. A dependency that skips a stage is written in the box instead (`build 4✓ ← lint`), so a line is never a guess; the tree view spells out every `(needs …)`.
- Matrix and reusable-workflow jobs are collapsed into one node showing counts (counts by status, e.g. `6✓ 1✗ 2●`); `enter` expands, `x` collapses.
- Dependencies come from the `needs:` entries of the workflow file at the run's commit. If they cannot be determined, gh-9s shows the plain jobs list and says why — it never draws a guessed graph.
- You get a terminal bell and a desktop notification when a run you are watching finishes.

**Pull request list** — `enter` detail · `u` opened by me · `e` review requested from me · `a` all · `o` browser

**Pull request detail** — description, review/merge state, checks summary, and the discussion in time order. `c` checks (`enter` on a GitHub Actions check opens that run's graph, in the run's own repo; other checks open in the browser) · `d` diff · `C` comment · `r` review (approve / request changes / comment) · `m` merge · `o` browser · `esc` back

**Diff** — added/removed lines coloured; `]`/`[` next/previous file · `f` file list (the same tree) · `/` search, `n`/`N` · `j/k`, `g/G`, `ctrl-d/u` scroll · `o` the PR in the browser. A diff over 5000 lines is cut and says so.

**Issue list** — `enter` open · `O`/`C`/`A` open/closed/all · `u` assigned to me · `m` opened by me · `o` browser

**Issue detail** — `C` comment · `x` close (asks first) · `X` reopen · `l` labels · `s` assignees · `o` browser

**Logs** — `j/k` scroll, `space`/`ctrl-d` page, `/` search (smart-case; `n`/`N` next/previous), `f` follow a running job

## Repo picker

`:repo` opens a picker: type to search, `tab` switches between **my access** (your own repos and your organisations') and **global** (all of GitHub; needs two characters and waits a moment after you stop typing), `enter` switches. The first row is the current directory's repo; with nothing typed, the repos you used recently are listed. Typed names still work: `:repo acme/api`.

## Reviewing and merging

Every dialog names the repo and number it acts on (`acme/api#482`). Approving, requesting changes and closing an issue ask for confirmation first; request changes also needs a reason. Long text can be written in your `$EDITOR` with `ctrl+e`.

**Merge** (`m` on a PR) reads the PR again, then judges it by GitHub's own merge state, not rules of its own:
- clean: pick a merge method (only the ones the repo allows are listed), then confirm — with a "delete branch after merge" checkbox whose last choice is remembered;
- unstable (non-required checks failing or still running): a warning, and `!` is required to go on;
- blocked, behind, conflicting, draft, closed, or still being computed: refused, with the reason.

`--admin` and auto-merge are never used.

## Trigger

`t` opens a form built from the workflow's declared `workflow_dispatch` inputs: text fields, dropdowns (`←/→`), booleans (`space`), and a branch picker. Required inputs block submission. After you submit, gh-9s finds the run that was created and opens its graph. If it cannot identify the run within 30s it says so and leaves you on the list; the workflow has still been dispatched.

## Project boards (read-only)

Pin a GitHub Project (v2) in `:fav` → Projects (`a`, then `OWNER/NUMBER`) and press `enter` to see its board: one tab per Status column in the Project's order (`All` first, `No status` when some card has none), the first 200 cards, with `enter` opening an issue or PR in its own repo (Enterprise hosts too), `o` the browser, `/` filter, `R` refresh. The Project's saved views show as a second row (`]` / `[` switch): each view's filter (`assignee:@me`, `label:`, `status:`, `is:open|pr|issue|draft|merged`, `repo:`, `no:`, words, `-` to exclude) and first sort (Title, Status, Updated) are applied to the loaded cards; anything else is named in a note instead of guessed, and roadmap views are not drawn. `b` switches to a real board: every Status as a column side by side (`h`/`l` change column, `j`/`k` change card, 24 columns wide at least, needs a terminal of 60 columns; the preview pane is not shown there). The preview pane works on the cards. Nothing is ever changed from here. Projects need the `read:project` permission: `gh auth refresh -s read:project` (add `--hostname HOST` for Enterprise).

## Favorites

A favorite is a workflow in a specific host/repo (`github.com/acme/api · deploy.yml`), stored in your personal config file (`~/Library/Application Support/gh-9s/favorites.json` on macOS, `~/.config/gh-9s/favorites.json` on Linux), never inside a repo. Give one an alias with `a`, then type `:<alias>` to jump to its runs. Aliases cannot shadow built-in commands. If the file is unreadable, favorites are disabled for the session and the file is left untouched.

Pinned repos and Projects live in `pins.json` next to it (`GH9S_PINS` overrides the path); a broken `pins.json` is never overwritten, pins then last for the session only.

## Project layout

```
main.go                 entry point (calls tui.Run)
internal/
  ui/                   shared state type, colours, status/time formatting
  workflow/             workflow YAML: dispatch inputs, job needs (pure, no network)
  graph/                job dependency graph, navigation, and the three renderers (+ golden files)
  diff/                 unified-diff parser (pure)
  ghcli/                the only code that runs `gh`: runs, jobs, logs, PRs, issues, merge, repos, search
  favorites/            favorites + aliases store, validation of the config file
  recent/               recently used repos, validated on load
  tui/                  Bubble Tea model, keys, screens (registered in screen.go), dialogs, picker, and the glue
docs/                   design spec and implementation plan
```

Only `ghcli` talks to GitHub; `workflow`, `graph`, `diff`, `favorites`, `recent` and `ui` are testable without a network, and the `tui` tests run with a stub `gh` first on `PATH`. New screens register themselves in their own file (`screen.go`) instead of growing the shared `switch` statements.

## Try the graph

`.github/workflows/graph-demo.yml` is a manual-only workflow that does no real work. It has six stages, three matrix jobs (unit 3×2, build 4, e2e 3), a skipped job, an `always()` job, and `workflow_dispatch` inputs of every kind the trigger form supports. Press `t` on it in gh-9s, or run `gh workflow run graph-demo.yml -f scenario=one-failure`, to watch a failure and its skipped downstream jobs.

## Design

Specs and implementation plans: `docs/superpowers/specs/` and `docs/superpowers/plans/` (release 1: Actions; release 2: pull requests, issues, repo picker).
