---
title: Reviewing a PR
description: Read diffs, navigate files, write inline comments and send a review.
---

Press `d` on a pull request to open the diff. Use it to read the change, comment on specific lines, and send a review.

## Layout

- On a terminal at least 120 columns wide, the diff is side by side: old on the left, new on the right, with changed words highlighted. The file list is on the left.
- Narrower terminals get a single-column diff.
- Press `s` to switch between the two by hand.

Code is coloured by language, chosen by file name, in both views. Coloring is computed per hunk, so a string or comment that starts above a hunk can be coloured wrongly. Unknown languages, lines over 400 characters, and very large diffs stay plain red and green.

A diff over 5000 lines is cut, and gh-9s says so.

## File tree

The file list is a directory tree.

- Directories come first.
- A chain of single-child directories is shown on one row, for example `internal/tui`.
- Each directory shows its totals.
- With Nerd Font icons, folders and files get an icon by language and kind.

| Key | Action |
|---|---|
| `tab` or `f` | Move focus between the file list and the diff |
| `j` / `k` / `g` / `G` | Move in the tree |
| `enter` or `space` | Fold or unfold a directory. On a file, jump to it. |
| `h` / `l` | Fold / unfold. On a file, `h` selects its directory. |
| `v` | Mark viewed (a file, or everything under a directory) |

Viewed marks are kept until the PR gets a new commit.

## Moving through the diff

| Key | Action |
|---|---|
| `]` / `[` | Next / previous file |
| `}` / `{` | Next / previous hunk |
| `/` then `n` / `N` | Search, then next / previous match |
| `j` / `k` | Move the cursor line by line |
| `g` / `G` | Top / bottom |
| `ctrl+d` / `ctrl+u` | Scroll half a page |
| `o` | Open the PR in the browser |

## Inline comments

A cursor (`▶`) walks through the diff with `j` and `k`.

| Key | Action |
|---|---|
| `c` | Write a comment on the cursor's line (the new side) |
| `C` | Write a comment on the old side of a changed or context line |
| `V` | Start a range. Move to the last line, then press `c`. `esc` cancels. |
| `x` | Remove the draft on the cursor's line |
| `X` | Discard all drafts |
| `A` | Adopt the drafts for the new commit (after a push) |

Comments other people left on lines appear under those lines. `◆n` in the file list counts them.

### Drafts

Your comments are **drafts** until you send them. A draft is shown under its line as "you (draft)".

- Drafts are saved to `drafts.json` next to your favorites file, per pull request. Closing gh-9s does not lose them.
- Each draft remembers the commit it was written for. After a push, drafts from the older commit are marked. Press `A` to keep them for the new commit, as long as the lines still exist.

## Sending the review

Press `r` to send your drafts.

1. Choose **approve**, **request changes**, or **comment**. The last two need a message.
2. Confirm.

Everything is sent as one GitHub review, pinned to the commit you were looking at.

If the PR has new commits since you opened the diff, nothing is sent. Press `R` to reload. Drafts whose lines no longer exist are reported and dropped.
