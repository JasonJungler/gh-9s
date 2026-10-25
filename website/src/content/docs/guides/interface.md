---
title: Interface basics
description: The layout, views, command bar, filter and global keys of gh-9s.
---

gh-9s runs inside any git repository with a GitHub remote. Every view shows data for that repository. You can switch to another repository at any time (see [Repositories](/gh-9s/guides/repos/)).

## Views

Seven views are one key away from anywhere:

| Key | Command | View |
|---|---|---|
| `1` | `:pr` (also `:pulls`, `:prs`) | Open pull requests. `*` marks the current branch. |
| `2` | `:issues` (also `:is`) | Issues. |
| `3` | `:runs` (also `:run`, `:r`) | Workflow runs. Refreshes every 5 s. |
| `4` | `:wf` (also `:workflows`, `:w`) | Workflows. Favorites are pinned on top with a ★. |
| `5` | `:discussions` (also `:disc`) | Discussions. |
| `6` | `:wiki` | Wiki pages. |
| `0` | `:inbox` (also `:in`) | Your inbox across repositories and hosts. |

`gh 9s` starts on the pull requests view. Change that with the `start_view` [setting](/gh-9s/guides/settings/). Outside a git repository with a GitHub remote there is no repo to show, so it starts on the inbox instead, with a note that `:repo OWNER/REPO` picks one.

`P` (or `:cpr`, `:checks`) opens the PR of the current branch, and `:fav` opens your favorites.

## Command bar

Press `:` to open the command bar under the header.

- It completes as you type. The rest of the best match appears in grey.
- `Tab` or `→` accepts the suggestion. Press `Tab` again to cycle through other matches.
- Completion covers built-in commands, your favorite aliases, and (after `:repo `) your recent and cached repositories.
- `Enter` runs a unique prefix, so `:wo` opens workflows.
- `↑` and `↓` walk through the commands you typed earlier.

See [Commands](/gh-9s/reference/commands/) for the complete list.

## Filter

`/` filters the list you are looking at. It is available on most lists, the inbox included. `esc` clears it.

## Global keys

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `g` / `G` | Top / bottom |
| `R` | Refresh |
| `o` | Open the selected item in your browser |
| `esc` | Back |
| `?` | Show every key of the current screen |
| `q` or `ctrl+c` | Quit |

Dialogs, forms and the repository picker open in a box at the top of the screen, like the command bar.

## First start: icons

The first time gh-9s starts, it shows a row of icons and asks whether you can see them.

- Answer `y` if your terminal uses a [Nerd Font](https://www.nerdfonts.com). Lists, checks, runs, the job graph, the inbox and the diff then show GitHub-style icons.
- Answer `n` to keep plain symbols.

Your answer is saved as the `icons` setting. Change it in `:settings`, run `:icons` to be asked again, or force one choice for a single run with `GH9S_ICONS=nerd` or `GH9S_ICONS=plain`.

Nothing is asked when the settings file cannot be used or when there is no terminal. Plain symbols are used then.

## Terminal width

gh-9s adapts to the width of your terminal:

- **120 columns or more:** side-by-side diffs, and the preview pane on the PR, issue and inbox lists.
- **About 100 columns of text or more:** PR and issue detail screens show a sidebar in a column on the right.
- **Narrower:** single-column diffs and full-width lists.

## Mouse

The mouse wheel scrolls any screen, three lines per notch. Clicks are not used. To select text in your terminal while the mouse is on, hold Option (iTerm2, Terminal.app) or Shift (Windows Terminal, tmux, most Linux terminals). Turn the wheel off with `:set mouse off`.
