# gh-9s

A [k9s](https://k9scli.io)-style terminal UI for GitHub Actions, as a `gh` extension.
Release 1 is workflow-focused: trigger workflows, watch runs as a live job graph, and keep your favorite workflows one keypress away.

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
| `3` / `:pr` | open pull requests (`*` marks the current branch) |
| `P` / `:cpr` | checks of the current branch's PR |
| `:fav` | favorite workflows from every repo, with the latest run's status |
| `:<alias>` | open the runs of the favorite with that alias |

## Keys

Everywhere: `j/k` move, `g/G` top/bottom, `/` filter, `:` command bar, `esc` back, `R` refresh, `o` open in browser, `q` quit.

**Runs** — `enter` job graph · `l` full log in pager · `L` failed log · `w` watch (`gh run watch`) · `r` rerun · `F` rerun failed · `c` cancel · `d` delete

**Workflows / favorites** — `enter` runs · `t` trigger form · `f` favorite/unfavorite · `a` set alias · `e` enable/disable

**Job graph** (watching a run)
- `hjkl` or arrows move between jobs, `enter` opens the job's log, `v` switches style, `ctrl-d/u` scroll
- Styles: **boxes** (default, left-to-right with arrows), **tree** (top-to-bottom, fits narrow terminals), **lanes** (timeline). Boxes too wide for the terminal fall back to the tree.
- Matrix and reusable-workflow jobs are collapsed into one node showing counts (`3/8 ✓`); `enter` expands, `x` collapses.
- Dependencies come from the `needs:` entries of the workflow file at the run's commit. If they cannot be determined, gh-9s shows the plain jobs list and says why — it never draws a guessed graph.
- You get a terminal bell and a desktop notification when a run you are watching finishes.

**Logs** — `j/k` scroll, `space`/`ctrl-d` page, `/` search (smart-case; `n`/`N` next/previous), `f` follow a running job

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
  ghcli/                the only code that runs `gh`: runs, jobs, logs, branches, trigger lookup
  favorites/            favorites + aliases store, validation of the config file
  tui/                  Bubble Tea model, keys, views, forms, and the glue between the packages
docs/                   design spec and implementation plan
```

Only `ghcli` talks to GitHub; `workflow`, `graph`, `favorites` and `ui` are testable without a network.

## Design

See `docs/superpowers/specs/2026-10-24-gh-9s-release1-design.md` and the implementation plan in `docs/superpowers/plans/`.
