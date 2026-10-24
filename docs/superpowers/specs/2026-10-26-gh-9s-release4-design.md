# gh-9s release 4 — Actions re-run/cancel and the inbox

Two independent changes, built in this order, one version each.

## Why

The user watches workflows and reviews other people's PRs from the terminal. After a red CI run the next step is always "run the failed jobs again" or "stop this run", and today that means leaving gh-9s. And every morning the question is "what is waiting for me?", which today means switching repo by repo. Success: a failed run can be re-run, or a running one stopped, without leaving the run's own screen; and one screen answers "what needs me" across repos.

Priority stated by the user: when the inbox opens, the first thing they want to see is **issues assigned to them**.

## Non-goals / anti-goals

- No notifications API (`/notifications`): its read/unread state and the thread model are a different feature. The inbox is built from searches.
- No bulk actions in the inbox (close, label, assign). It opens things; the existing screens act on them.
- Nothing that acts on a run without a confirmation naming `owner/repo` and the run (this includes the existing `r` and `F`, which today act at once).
- No re-run of runs that are still in progress, and no cancel of finished ones: those keys say why they do nothing.

## Part 1 — safer, fuller re-run and cancel

These already exist on the run list: `r` re-runs the run, `F` re-runs the failed jobs (both act at once, with no confirmation), `c` cancels (after a "cancel run N? (y/N)" that does not name the repo), `d` deletes (confirmed). This part keeps those keys and makes them safe and available where the run is being looked at. (The user decided, on being asked: `r` and `F` get a confirmation.)

- **Same keys, now confirmed:** `r`, `F`, `c`, `d` on the run list ask first in the same dialog style as the PR actions: "Re-run the failed jobs of run 123 in acme/api?" — the repo is always named. Each acts on the repo and run the screen was opened for (not the chosen base repo, not whatever is on screen after navigating away), exactly like the PR actions.
- **Say why not:** `F` on a run with nothing failed ("nothing failed to re-run"), `r`/`F` on a run still in progress, `c` on a finished run — a one-line status message and no dialog.
- **After a re-run is accepted** the screen switches to that run's graph and keeps watching (same run id, higher attempt). The "finished" notification fires again for the new attempt.
- **Also in the places the run is shown:** on the job graph and the job list the same keys work for the run being shown, with the same dialogs and rules (the builder resolves clashes with the graph's existing keys, e.g. `x` collapses a group, and records them; existing keys keep working, so the graph may use other letters).
- Errors from `gh` ("run is too old to re-run", no permission) are shown as they are. Enterprise hosts work as everywhere (`-R HOST/OWNER/REPO`).
- **Jump to the failure:** in the job graph, a key (builder's call) opens the log of the first failed job, scrolled to its first failed step when the log makes that findable, else to the first line matching `error|failed`.
- The README's key table is updated.

## Part 2 — the inbox

- A new screen, `:inbox` (also `0`), listed in the hint line. Tabs, switched with `tab`/`shift+tab` or `1`–`4` inside the screen, in this order and opening on the first:
  1. **Assigned issues:** open issues assigned to me, any repo.
  2. **Review requested:** open PRs where my review is requested.
  3. **My PRs:** open PRs I authored, with the review decision and check state shown.
  4. **Mentions:** open issues and PRs that mention me.
- Each row: repo, number, title, author, last update; PR rows also show checks and review decision. Sorted by last update, newest first. A count on each tab label.
- `enter` opens the item in the existing PR detail / issue detail (with all its actions and the review screen); `esc` returns to the inbox on the same tab and row. `o` opens in the browser. `R` refreshes. The list refreshes by itself every minute while the screen is open, and a row that was not there before is marked as new until the user leaves the tab.
- Data comes from GitHub's search, one query per tab per host, not from per-repo calls. `gh search prs` cannot return the review decision or the check state, so the queries go through `gh api graphql` (`search(type: ISSUE, query: …)`), which returns both with the results in one call. (My call, to stay within the search rate limit and to show those two columns.) The number of rows per tab is capped (100, with "more on GitHub" shown when capped).
- Hosts: the host `gh` is logged in to by default, plus every other host `gh` is logged in to (Enterprise), each queried separately and merged; a host that fails (offline, expired token) gives a one-line note on the screen and the other hosts still show. Rows show the host when it is not github.com.
- Filtering with `/` works as on the other lists. Opening an item sets the repo/target the way the other screens do (the item's own repo, not the current directory's).
- Empty tab: a clear sentence ("nothing assigned to you") rather than a blank table.

## Constraints

- Go, Bubble Tea v1, `gh` for all GitHub access; all outside calls behind package-level seams; tests never touch the network.
- TDD, a fresh-reviewer pass per part, the finishing menu after each; no push/tag without the user's approval. README updated.
- Existing keys keep working.

## Left to the builder

Exact key letters where the spec says "a key"; the shape of the search queries and JSON fields; how hosts are discovered; layout of columns; the refresh mechanics; how "new" is remembered (in memory is enough).
