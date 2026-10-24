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
| `:fav` | favorite workflows from every repo, with the latest run's status |
| `:<alias>` | open the runs of the favorite with that alias |
| `:repo` | open the repo picker (below) |
| `:repo OWNER/REPO` | switch every view to another repo (`HOST/OWNER/REPO` for another host). The repo is checked first. The current-branch PR (`P`, `:cpr`) only exists in the current directory's repo. |

## Keys

Everywhere: `j/k` move, `g/G` top/bottom, `/` filter, `:` command bar, `esc` back, `R` refresh, `o` open in browser, `q` quit.

Links (`o`) open in your browser: `$BROWSER` if set, otherwise the system default. Under WSL they open in the Windows browser (`wslview` if installed, else `cmd.exe /c start`). If it can't be opened, the reason is shown in the status line.

PR and issue descriptions and comments are rendered as formatted markdown (headings, lists, task lists, tables, code blocks, quotes). Images show as `[image: alt]` markers: `tab` / `shift+tab` select one, `enter` opens it full screen (`esc` or `q` to come back, `o` to open it in the browser). The picture is drawn with iTerm2's inline images in iTerm2, Sixel in Windows Terminal, and coloured half-block characters everywhere else (macOS Terminal, VS Code, ssh). Inside tmux, graphics are used only when tmux's `allow-passthrough` is on; otherwise you get the half-block picture. `GH9S_IMAGES=auto|iterm|sixel|blocks|off` overrides the choice (`off` sends images to the browser). Images are fetched with your `gh` login (only from your GitHub host), up to 5 MB, as png, jpeg or gif.

**Reviewing a PR** (`d` on a PR): on a terminal at least 120 columns wide the diff is shown side by side — old on the left, new on the right, changed words highlighted — with the file list on the left. Narrower terminals get the single-column diff; `s` switches by hand. `tab` moves between the file list and the diff, `]` / `[` jump between files, `}` / `{` between hunks, `v` marks a file viewed (kept until the PR gets a new commit), `/` searches. Comments other people left on lines of the diff appear under those lines (`◆n` in the list counts them).

The `:` command bar, `/` filter, log search and alias entry open in a box under the header, like k9s. The command bar completes as you type: the rest of the best match shows in grey, `Tab` (or `→`) accepts it and `Tab` again cycles through the other matches. Commands complete from the built-ins, your favorite aliases and (after `:repo `) your recent and cached repos. `Enter` on a unique prefix runs it (`:wo` opens workflows), and `↑`/`↓` walk the commands you typed before. Dialogs, forms and the repo picker are boxed at the top too.

**Runs** — `enter` job graph · `l` full log in pager · `L` failed log · `w` watch (`gh run watch`) · `r` rerun · `F` rerun failed · `c` cancel · `d` delete

**Workflows / favorites** — `enter` runs · `t` trigger form · `f` favorite/unfavorite · `a` set alias · `e` enable/disable

**Job graph** (watching a run)
- `hjkl` or arrows move between jobs, `enter` opens the job's log, `v` switches style, `ctrl-d/u` scroll
- Styles: **boxes** (default, left-to-right with arrows), **tree** (top-to-bottom, fits narrow terminals), **lanes** (timeline). Boxes too wide for the terminal fall back to the tree.
- In the boxes style, lines connect only neighbouring stages. A dependency that skips a stage is written in the box instead (`build 4✓ ← lint`), so a line is never a guess; the tree view spells out every `(needs …)`.
- Matrix and reusable-workflow jobs are collapsed into one node showing counts (counts by status, e.g. `6✓ 1✗ 2●`); `enter` expands, `x` collapses.
- Dependencies come from the `needs:` entries of the workflow file at the run's commit. If they cannot be determined, gh-9s shows the plain jobs list and says why — it never draws a guessed graph.
- You get a terminal bell and a desktop notification when a run you are watching finishes.

**Pull request list** — `enter` detail · `u` opened by me · `e` review requested from me · `a` all · `o` browser

**Pull request detail** — description, review/merge state, checks summary, and the discussion in time order. `c` checks (`enter` on a GitHub Actions check opens that run's graph, in the run's own repo; other checks open in the browser) · `d` diff · `C` comment · `r` review (approve / request changes / comment) · `m` merge · `o` browser · `esc` back

**Diff** — added/removed lines coloured; `]`/`[` next/previous file · `f` file list · `/` search, `n`/`N` · `j/k`, `g/G`, `ctrl-d/u` scroll · `o` the PR in the browser. A diff over 5000 lines is cut and says so.

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

## Favorites

A favorite is a workflow in a specific host/repo (`github.com/acme/api · deploy.yml`), stored in your personal config file (`~/Library/Application Support/gh-9s/favorites.json` on macOS, `~/.config/gh-9s/favorites.json` on Linux), never inside a repo. Give one an alias with `a`, then type `:<alias>` to jump to its runs. Aliases cannot shadow built-in commands. If the file is unreadable, favorites are disabled for the session and the file is left untouched.

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
