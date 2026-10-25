---
title: Pull requests
description: List, filter, inspect, check, review and merge pull requests.
---

The pull request view (`3` or `:pr`) lists open pull requests in the current repository. Press `enter` to open one.

## List

| Key | Action |
|---|---|
| `enter` | Open the pull request |
| `u` | Show only the PRs opened by you |
| `e` | Show the PRs that requested your review |
| `a` | Show all open PRs |
| `f` | Pin or unpin this repository |
| `o` | Open in the browser |

The CHECKS column is loaded separately, because GitHub can take many seconds to compute the checks of 50 PRs on a large repository. Until it arrives the column shows `…`. If it fails, the column shows `-` and the status line says why.

On a terminal 120 columns or wider, a preview pane on the right shows the selected PR. See [Preview pane](#preview-pane).

## Detail screen

Press `enter` on a PR to open its detail screen. It shows:

- The description, rendered as Markdown. See [Images and Markdown](/gh-9s/guides/images-and-markdown/).
- Review and merge state.
- A summary of the checks.
- The discussion, in time order.

On a terminal about 100 columns of text wide or more, a sidebar appears on the right with reviewers and their status, assignees, labels, projects, milestone and the issues the PR closes.

| Key | Action |
|---|---|
| `c` | Show the checks |
| `d` | Open the diff and code review. See [Reviewing a PR](/gh-9s/guides/reviewing-prs/). |
| `C` | Write a comment |
| `r` | Review: approve, request changes, or comment |
| `m` | Merge |
| `o` | Open in the browser |
| `esc` | Back |

## Checks

Press `c` to list the checks of the PR.

- `enter` on a GitHub Actions check opens that run's [job graph](/gh-9s/guides/job-graph/), in the run's own repository.
- Other checks open in the browser.

The checks view is also available as `P` or `:cpr` for the PR of the current branch.

## Review

Press `r` to choose one of three reviews:

- **Approve**
- **Request changes**: needs a reason.
- **Comment**: needs a message.

Each review asks for confirmation. Long text can be written in your `$EDITOR` with `ctrl+e`.

If you have inline comments in drafts, they are sent together with the review. See [Reviewing a PR](/gh-9s/guides/reviewing-prs/).

## Merge

Press `m` on a PR. gh-9s reads the PR again and judges it by GitHub's own merge state. It does not apply its own rules.

| Merge state | What happens |
|---|---|
| **Clean** | You pick a merge method. Only the methods the repository allows are listed. Then confirm. The "delete branch after merge" checkbox starts from your setting and remembers your last choice. |
| **Unstable** (non-required checks failing or still running) | A warning. You must type `!` to continue. |
| **Blocked, behind, conflicting, draft, closed, or still being computed** | Refused, with the reason. |

gh-9s never uses `--admin` and never enables auto-merge.

## Preview pane

On the PR list, on a terminal 120 columns or wider, a pane on the right shows the selected PR:

- Title, author and `base ← head`.
- Labels, review decision, and check counts with the names of the failing checks.
- Size and description.
- The latest comments.

The detail loads after the selection has rested for a moment, so holding `j` does not send a request for every row. Details are kept for five minutes (up to 50 items).

When the pane is wide enough (about 72 columns of text), the PR's sidebar sits to the right of the description. In a narrower pane it comes above the description.

| Key | Action |
|---|---|
| `J` / `K` | Scroll the pane |
| `p` | Hide or show the pane for this session |
| `enter` | Open the full screen |

The settings `preview`, `preview_width` and `preview_min_columns` control the pane. See [Settings](/gh-9s/guides/settings/).
