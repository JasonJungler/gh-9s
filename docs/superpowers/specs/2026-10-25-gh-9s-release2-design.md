# gh-9s release 2: pull requests, issues, and an interactive repo picker

## Intent

Release 1 made gh-9s a k9s-style terminal UI for GitHub Actions. Release 2 widens it to the rest
of the daily GitHub loop so the owner can stay in the terminal: look at pull requests (and jump
from a failing check straight into the Actions run), review other people's PRs including reading
the diff, merge, comment, manage issues, and move between repos without leaving the app.

## Starting point (already built, on `main`)

Go + Bubble Tea app that shells out to the `gh` CLI. Packages under `internal/`: `ui` (state and
colours), `workflow` (YAML parsing), `graph` (job graph + three renderers), `ghcli` (the only code
that runs `gh`), `favorites`, `tui` (model, keys, views). Existing relevant behaviour:
- Views: runs, workflows, PRs (list), PR checks, jobs/graph, logs, favorites.
- `:repo OWNER/REPO` verifies a repo with `gh` and points every view at it; `:repo` alone returns to
  the current directory's repo. The model has `base` (the chosen repo) and `target` (what the view
  shows; differs from `base` only while visiting a favorite).
- Command bar (`:`), filter (`/`), trigger form, confirm prompts, a log viewer with search.
- Views are dispatched by `switch` statements spread over `cmds.go`, `keys.go`, `update.go`,
  `view.go`; adding a view currently touches all of them.
Release 2 builds on this code; it does not rewrite it.

## Goals (in priority order)

1. **Interactive repo picker.** `:repo` with no argument opens a picker.
2. **PR browsing, checks, and the jump into Actions.**
3. **PR diff in the terminal.**
4. **PR actions:** comment, review (approve / request changes / comment), merge.
5. **Issue browsing and commenting.**
6. **Issue management:** close, reopen, labels, assignees.

## Non-goals for release 2

- Inline (per-line) review comments. The first version comments on the PR as a whole only.
- Creating issues or PRs; editing titles or bodies; milestones, projects, reactions.
- Auto-merge, merge queues, branch protection administration.
- Checking out a PR locally.
- A host switcher (hosts still come from `gh`'s login / `GH_HOST`).

## Anti-goals

- No irreversible action (merge, request changes, close) without an explicit confirmation step.
- No chatty API use: search is debounced, lists are fetched on demand and cached for a short time,
  no continuous polling of PR/issue views beyond what release 1 already does for runs.
- No silent wrong target: every action names the repo and PR/issue number it will act on.
- No network in unit tests.

## Decisions made with the owner

### Repo picker
- `:repo` with no argument opens a picker (a centered panel). `:repo OWNER/REPO` keeps working.
- Typing searches as you type (debounced). The scope is shown top-right and toggled with `tab`:
  **repos I have access to** (the owner's own plus the organisations they belong to) is the default;
  **global** searches all of GitHub.
- Selecting a repo with `enter` switches to it exactly as `:repo` does today (verified first).
- The first row is "the current directory's repo". With an empty query the picker shows recently used
  repos.
- Recently used repos are remembered across sessions.

### Pull requests
- The PR list can be filtered to PRs I opened and PRs where my review is requested.
- Pressing `enter` on a check that is a GitHub Actions job or run opens that run's graph view in the
  PR's repo (same graph, logs and watch as release 1); `esc` returns to the PR. Checks that are not
  Actions (external CI, status contexts) open in the browser instead.
- The diff is readable in the terminal: coloured added/removed lines, scrolling, a file list and
  next/previous-file keys, and search with the same keys as the log viewer.
- Actions available on a PR: comment on the PR, submit a review (approve, request changes, or
  comment), and merge.
- Comments and review bodies are PR-level only in this release.

### Issues
- Issue list with filters for open/closed and for issues assigned to me / opened by me.
- Viewing an issue shows its body and comments.
- Actions: comment, close, reopen, add/remove labels, assign/unassign. When adding a label or an
  assignee the user picks from the repo's existing labels / assignable users instead of typing names.

## Designer's suggestions awaiting the owner's confirmation

These came up while designing and have not been confirmed; the builder should implement them as
described unless the owner says otherwise.
- **Merge dialog.** `m` on a PR opens a dialog showing checks, review state and conflicts; it lists
  only the merge methods the repo allows (merge commit / squash / rebase) and has a
  "delete branch after merge" checkbox that remembers the last choice. If a rule is not satisfied
  (failing or pending checks, missing approvals) the dialog does not allow a plain merge; the user
  must deliberately use a separate "merge anyway" key.
- **Review/comment dialogs** follow the trigger form's interaction style: pick the action, type the
  text, confirm.

## Constraints

- Language and UI stack unchanged (Go, Bubble Tea, lipgloss); no new third-party dependencies unless
  unavoidable.
- All GitHub access goes through the `gh` CLI, in package `ghcli`. Enterprise follows `gh`'s login.
- Release 1's rules still hold: pure logic in packages without network, `-race` tests green, goldens
  for rendering.
- Everything must work against any repo chosen with `:repo`, not only the current directory's.

## Left to the builder (the designer's calls, open to change)

- Keys for each new action (choose consistently with existing ones; list them in the footer).
- Whether to introduce a small registry so views register their fetch/keys/render instead of
  growing the existing `switch` statements.
- How multi-line comment and review text is entered (suggested: a single-line box plus a key to
  compose in `$EDITOR`).
- How the repos-with-access list is built and cached (suggested: the user's repos plus each
  organisation's, fetched once per session and filtered locally) and the global-search minimum query
  length and debounce interval (search API limits are tight).
- Where recently used repos are stored (suggested: next to `favorites.json`).
- Diff handling for very large diffs and binary/renamed files (suggested: cap the lines shown and
  offer to open in the browser beyond the cap).
- How to word and confirm destructive actions.

## Calls made after the builder check (the designer's, open to change)

1. **"Merge anyway" never bypasses protection.** The merge dialog uses GitHub's own verdict
   (`mergeable`, `mergeStateStatus`, `reviewDecision` from `gh pr view --json`), not rules invented
   by the app. A plain merge is offered when GitHub says the PR is mergeable and clean. When GitHub
   reports it as unstable (for example, non-required checks failing or pending) the dialog warns and
   needs the deliberate "merge anyway" key, which still just attempts a normal merge. When GitHub
   reports it blocked (required checks or approvals missing, conflicts) the merge is not offered,
   because GitHub would refuse it. `gh pr merge --admin` is never passed in release 2.
2. **There is a PR detail screen.** `enter` on a PR in the list opens it (this replaces today's
   `enter` going straight to checks). It shows the title, author, branches, review decision, checks
   summary and mergeability, the description, and the existing comments and reviews. From it:
   `c` checks (the existing checks view, with the jump into Actions), `d` diff, `m` merge,
   `r` review, and a key to comment. `esc` from the checks, diff or an Actions run returns to this
   screen. Reading existing discussion before commenting or reviewing is the point.
3. **Polling.** What release 1 polls keeps polling (runs, the PR list, PR checks). The new screens
   (PR detail, diff, issue list, issue detail, picker) do not poll; `R` refreshes them and every
   action refreshes the affected screen afterwards.
4. **Filters are per repo.** "Opened by me" and "review requested from me" apply to the repo being
   shown (the current directory's, or the one chosen with `:repo`/the picker). "Review requested"
   uses GitHub's `review-requested:@me`, which includes requests made to a team the user is on.
   A cross-repo "everything waiting on me" inbox is not in this release.
5. **Target vs base.** New views and actions use whatever repo the current view shows (`target`),
   so they stay correct while a favorite from another repo is open. The picker's "recent" list
   records only repos chosen with the picker or `:repo`, not favorite visits.
6. **Hosts.** The picker searches the host of the repo being shown (or `gh`'s default host when
   there is none). Picker rows show the host only when it is not github.com. Searching all logged-in
   hosts at once is out of scope.
7. **Delivery in stages.** The goal order is a cut line: each goal is merged as soon as it is green
   and the next one starts, so release 2 can be tagged after any of them. Because four new view
   kinds follow, the first task introduces the small view registry so each view lives in its own
   file instead of growing the existing `switch` statements.
