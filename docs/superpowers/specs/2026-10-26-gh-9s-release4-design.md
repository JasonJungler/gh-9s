# gh-9s release 4 — Actions re-run/cancel and the inbox

Two independent changes, built in this order, one version each.

## Why

The user watches workflows and reviews other people's PRs from the terminal. After a red CI run the next step is always "run the failed jobs again" or "stop this run", and today that means leaving gh-9s. And every morning the question is "what is waiting for me?", which today means switching repo by repo. Success: a failed run can be re-run, or a running one stopped, without leaving the run's own screen; and one screen answers "what needs me" across repos.

Priority stated by the user: when the inbox opens, the first thing they want to see is **issues assigned to them**.

## Non-goals / anti-goals

- No notifications API (`/notifications`): its read/unread state and the thread model are a different feature. The inbox is built from searches.
- No bulk actions in the inbox (close, label, assign). It opens things; the existing screens act on them.
- Nothing that acts on a run without a confirmation naming `owner/repo` and the run.
- No re-run of runs that are still in progress, and no cancel of finished ones: those keys say why they do nothing.

## Part 1 — re-run and cancel runs

- On the run list, the job graph and the job list: `e` re-runs the failed jobs of the run, `E` re-runs the whole run, `x` cancels a run that is in progress (queued or running). (Check these keys against the existing ones on each screen and keep every existing key working; the builder resolves conflicts and records them.)
- Each asks for confirmation first ("Re-run the failed jobs of run 123 in acme/api?"). The action uses the repo and run the screen was opened for (the same identity rules as the PR actions: not the chosen base repo, not whatever is on screen after navigating away).
- After a re-run is accepted the screen switches to that run's graph and keeps watching (the run id is the same; its attempt number goes up). The watch notification for "finished" fires again for the new attempt.
- Why it does nothing: `e` on a successful run ("nothing failed to re-run"), `E` on a run in progress, `x` on a finished run — a one-line status message, no dialog.
- Errors from `gh` (for example "run is too old to re-run", or no permission) are shown as they are.
- Enterprise hosts work the same way as everything else (`-R HOST/OWNER/REPO`).
- Jump to the failure: in the job graph, a key (the builder's call) opens the log of the first failed job with the view already scrolled to its first failed step if the log makes that findable, else to the first line matching `error|failed`.

## Part 2 — the inbox

- A new screen, `:inbox` (also `0`), listed in the hint line. Tabs, switched with `tab`/`shift+tab` or `1`–`4` inside the screen, in this order and opening on the first:
  1. **Assigned issues:** open issues assigned to me, any repo.
  2. **Review requested:** open PRs where my review is requested.
  3. **My PRs:** open PRs I authored, with the review decision and check state shown.
  4. **Mentions:** open issues and PRs that mention me.
- Each row: repo, number, title, author, last update; PR rows also show checks and review decision. Sorted by last update, newest first. A count on each tab label.
- `enter` opens the item in the existing PR detail / issue detail (with all its actions and the review screen); `esc` returns to the inbox on the same tab and row. `o` opens in the browser. `R` refreshes. The list refreshes by itself every minute while the screen is open, and a row that was not there before is marked as new until the user leaves the tab.
- Data comes from `gh search issues` / `gh search prs` (assignee, review-requested, author, mentions, state open), not from per-repo calls; the number of rows per tab is capped (100, with "more on GitHub" shown when capped).
- Hosts: the host `gh` is logged in to by default, plus every other host `gh` is logged in to (Enterprise), each queried separately and merged; a host that fails (offline, expired token) gives a one-line note on the screen and the other hosts still show. Rows show the host when it is not github.com.
- Filtering with `/` works as on the other lists. Opening an item sets the repo/target the way the other screens do (the item's own repo, not the current directory's).
- Empty tab: a clear sentence ("nothing assigned to you") rather than a blank table.

## Constraints

- Go, Bubble Tea v1, `gh` for all GitHub access; all outside calls behind package-level seams; tests never touch the network.
- TDD, a fresh-reviewer pass per part, the finishing menu after each; no push/tag without the user's approval. README updated.
- Existing keys keep working.

## Left to the builder

Exact key letters where the spec says "a key"; the shape of the search queries and JSON fields; how hosts are discovered; layout of columns; the refresh mechanics; how "new" is remembered (in memory is enough).
