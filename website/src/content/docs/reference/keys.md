---
title: Keys
description: Every keyboard shortcut in gh-9s, by screen.
---

Press `?` on any screen to see its keys inside gh-9s. This page lists them all.

## Everywhere

| Key | Action |
|---|---|
| `1` `2` `3` `4` `5` `6` `0` | Runs, workflows, pulls, issues, discussions, wiki, inbox |
| `:` | Command bar |
| `/` | Filter the list |
| `P` | The PR of the current branch |
| `R` | Refresh |
| `esc` | Back |
| `?` | Help for this screen |
| `q` or `ctrl+c` | Quit |

Scrolling works the same on every screen that scrolls:

| Key | Action |
|---|---|
| `j` / `k` or `↓` / `↑` | One line |
| `ctrl+d` / `ctrl+u` | Half a page |
| `space` / `pgdn` / `pgup` | A page |
| `g` / `G` or `home` / `end` | Top / bottom |

## Runs

| Key | Action |
|---|---|
| `enter` | Job graph of the run |
| `l` | Full log in the pager |
| `L` | Log of the failed jobs |
| `f` | Pin or unpin this repository |
| `w` | Watch the run |
| `r` | Re-run (asks first) |
| `F` | Re-run the failed jobs |
| `c` | Cancel |
| `d` | Delete (asks first) |
| `o` | Open in the browser |

## Job graph

| Key | Action |
|---|---|
| `h` `j` `k` `l` or arrows | Move between jobs |
| `enter` | Log of the job, or expand a matrix |
| `x` | Collapse |
| `v` | Change the graph style |
| `f` | Log of the first failed job, at its first error |
| `r` `F` `c` `d` | Re-run, re-run failed, cancel, delete |
| `o` | Open in the browser |
| `ctrl+d` / `ctrl+u` | Scroll |

## Workflows

| Key | Action |
|---|---|
| `enter` | Runs of the workflow |
| `t` | Trigger (form) |
| `f` | Favorite or unfavorite |
| `a` | Set an alias |
| `e` | Enable or disable |
| `o` | Open in the browser |

## Favorites

| Key | Action |
|---|---|
| `tab` / `l` | Next tab |
| `shift+tab` / `h` | Previous tab |

Repos tab: `enter` switch, `a` pin, `f` or `d` unpin (asks first), `o` open.

Projects tab: `enter` open the board, `a` pin, `f` or `d` unpin (asks first), `o` open.

## Pull requests (list)

| Key | Action |
|---|---|
| `enter` | Open the PR |
| `u` | Only mine |
| `e` | Review requested from me |
| `a` | All |
| `f` | Pin or unpin this repository |
| `o` | Open in the browser |

## Pull request (detail)

| Key | Action |
|---|---|
| `c` | Checks |
| `d` | Diff and code review |
| `C` | Comment |
| `r` | Review: approve, request changes, or comment |
| `m` | Merge |
| `M` | Add to / remove from Projects (asks first) |
| `tab` | Select an image |
| `enter` | Show the selected image |
| `o` | Open the image, or the PR |

## Diff

| Key | Action |
|---|---|
| `]` / `[` | Next / previous file |
| `}` / `{` | Next / previous hunk |
| `/` then `n` / `N` | Search |
| `tab` or `f` | Focus the file tree (side by side) or the file list (single column) |
| `s` | Side by side / single column |
| `v` | Mark viewed |
| `c` | Comment on the line (new side) |
| `C` | Comment on the old side |
| `V` | Start a range, move, then `c` |
| `x` | Remove the draft on this line |
| `X` | Discard all drafts |
| `A` | Adopt the drafts for the new commit |
| `r` | Send the review (asks first) |
| `esc` | Cancel a range, or return to the diff |
| `o` | Open the PR in the browser |

In the file tree:

| Key | Action |
|---|---|
| `j` `k` `g` `G` | Move |
| `enter` or `space` | Fold or unfold a directory. On a file, jump to it. |
| `h` / `l` | Fold / unfold. On a file, `h` selects its directory. |

## Checks

| Key | Action |
|---|---|
| `j` / `k` | Move |
| `enter` or `o` | Open the check |

## Jobs

| Key | Action |
|---|---|
| `enter` | Log of the job |
| `r` `F` `c` `d` | Re-run, re-run failed, cancel, delete the run |
| `o` | Open |

## Issues (list)

| Key | Action |
|---|---|
| `enter` | Open the issue |
| `a` | Send to a coding agent (asks first) |
| `O` / `C` / `A` | Open / closed / all |
| `u` | Assigned to me |
| `m` | Opened by me |
| `M` | Add to / remove from Projects (asks first) |
| `f` | Pin or unpin this repository |
| `o` | Open in the browser |

## Issue (detail)

| Key | Action |
|---|---|
| `a` | Send to a coding agent (asks first) |
| `C` | Comment |
| `x` | Close (asks first) |
| `X` | Reopen |
| `l` | Labels |
| `s` | Assignees |
| `tab` / `enter` | Select an image / show it |
| `o` | Open |

## Discussions (list)

| Key | Action |
|---|---|
| `enter` | Open the discussion |
| `c` | Narrow to a category (or all) |
| `o` | Open in the browser |

## Discussion (detail)

| Key | Action |
|---|---|
| `n` / `N` | Select the next / previous comment or reply |
| `C` | Comment (asks first) |
| `r` | Reply in the selected comment's thread (asks first) |
| `o` | Open in the browser |

## Wiki (list)

| Key | Action |
|---|---|
| `enter` | Read the page |
| `e` | Edit the page in `$EDITOR`, then ask for a commit message |
| `n` | New page (asks for the title, then opens the editor) |
| `x` | Delete the page (asks first) |
| `/` | Filter by title |
| `o` | Open the page, or the wiki, in the browser |
| `R` | Update from GitHub |

## Wiki page

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Select the next / previous link to another page |
| `enter` | Follow the selected link |
| `esc` | Back along the links followed, then to the list |
| `e` | Edit in `$EDITOR`, then ask for a commit message |
| `x` | Delete the page (asks first) |
| `o` | Open in the browser |
| `R` | Update from GitHub |

## Inbox

| Key | Action |
|---|---|
| `tab` / `l` | Next tab |
| `shift+tab` / `h` | Previous tab |
| `enter` | Open the item |
| `o` | Open in the browser |
| `R` | Refresh (at most once in a while) |

## Project board

| Key | Action |
|---|---|
| `tab` / `l` / `→` | Next column |
| `shift+tab` / `h` / `←` | Previous column |
| `j` / `k` | Move |
| `enter` | Open the issue or PR |
| `s` | Change the Status (asks first) |
| `H` / `L` | Move the card to the previous / next column (column board) |
| `a` | Change assignees |
| `n` | Add an issue or PR |
| `x` / `X` | Remove the card from the Project / archive it |
| `N` | New draft card (gets the Status of the column you are in) |
| `e` | Edit a draft card's title and text |
| `v` | Change another field of the card |
| `space` | Mark the card (`●`); `s` `a` `x` `X` `H` `L` then act on all marked cards. `esc` clears the marks |
| `ctrl+↑` / `ctrl+↓` (or `alt+↑` / `alt+↓`) | Move the card one place up / down (not in a sorted view) |
| `u` | Undo the last Status or field change, added card or new draft |
| `b` | Switch between list and column board |
| `h` `j` `k` `l` | In the column board: change column / card |
| `]` / `[` | Next / previous saved view |
| `f` | Pin or unpin this Project |
| `o` | Open in the browser |
| `R` | Refresh (at most once in a while) |

## Logs

| Key | Action |
|---|---|
| `j` / `k` | Scroll |
| `/` then `n` / `N` | Search |
| `f` | Follow a running job |
| `o` | Open |

## Preview pane

| Key | Action |
|---|---|
| `J` / `K` | Scroll the pane |
| `p` | Hide or show the pane for this session |

## Settings

| Key | Action |
|---|---|
| `j` / `k` | Move |
| `enter` or `→` | Next value |
| `←` | Previous value |
