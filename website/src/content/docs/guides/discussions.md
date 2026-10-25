---
title: Discussions
description: List a repository's Discussions by category, read them with their replies, comment and reply.
---

The Discussions view (`5` or `:discussions`, short form `:disc`) lists the Discussions of the current repository, the ones with the latest activity first. Press `enter` to open one. If the repository has Discussions turned off, the view says so.

## List

| Key | Action |
|---|---|
| `enter` | Open the discussion |
| `c` | Narrow the list to one category, or back to all |
| `o` | Open in the browser |

A `✓` before the title marks a discussion that has an answer; `[closed]` and `[locked]` are marked too. The list shows the first 50; when there are more, the status line says "first 50 of N", and `c` is how to get to the rest. The category choice is read on GitHub's side, so the first 50 are of that category. Switching repository forgets the category.

On a terminal 120 columns or wider the selected discussion's text is shown in a pane on the right, like the issue list.

## One discussion

The screen shows the text, then the comments, each with its replies under it (GitHub nests replies one level deep). The comment marked as the answer shows `✓ answer`. Up to 100 comments and 50 replies per comment are read; when there are more, a line says how many are not shown, and `o` opens the discussion in the browser.

| Key | Action |
|---|---|
| `j` / `k` | Scroll |
| `n` / `N` | Select the next / previous comment or reply (marked `▌` in the margin) |
| `C` | Write a new comment (asks first) |
| `r` | Reply to the selected comment (asks first); a reply to a reply goes into the same thread |
| `o` | Open in the browser |
| `R` | Read it again |

The question for a comment or reply names the repository and the number, and a reply names whom it answers. A locked discussion cannot be written to, and the screen says so instead of asking. Everything GitHub sends is cleaned of control characters before it is drawn.

Reading and writing use the `repo` permission that `gh` has by default; no extra scope is needed. Enterprise hosts work the same way as everywhere else in gh-9s.

Not in this release: starting or editing a discussion, marking an answer, closing or locking, voting, and selecting images in the text.
