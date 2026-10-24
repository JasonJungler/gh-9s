# gh-9s release 1: workflow-focused TUI

## Intent

gh-9s is a `gh` CLI extension (`gh 9s`) that gives GitHub a k9s-style
terminal UI. The long-term goal is to cover most of GitHub (pull requests,
issues, repo search) interactively. **Release 1 is deliberately limited to
GitHub Actions workflows**: the owner wants to trigger workflows, watch runs,
and keep favorite workflows one keypress away, all without leaving the
terminal. Pull requests, issues and repo search come in later releases.

## Starting point (already built and pushed)

Go + Bubble Tea app, shelling out to the `gh` CLI for all data and actions
(so auth and GitHub Enterprise come from `gh`). Existing views: runs,
workflows, PRs, PR checks, jobs, and a built-in log viewer with follow mode.
Existing actions: trigger with free-typed `ref key=value`, watch (via
`gh run watch`), rerun, rerun failed, cancel, delete, enable/disable, open in
browser. Repo: github.com/MurabitoB/gh-9s. Tool versions are pinned with mise.
Release 1 builds on this code; it does not rewrite it.

## Goals (in priority order)

1. **Favorite workflows.** The user can mark workflows as favorites, give each
   an alias, and reach them quickly.
2. **Smarter trigger.** A form for dispatching a workflow, built from its
   declared inputs.
3. **Trigger then watch.** Submitting the trigger form opens the new run's
   job graph immediately.
4. **Job graph for watching a run.** The run's jobs are drawn as a live graph.
5. **Log search.** Search inside the log viewer.
6. **Run-finished notification.** A signal when a watched run completes.

## Non-goals for release 1

- Pull request, issue, release, or repo-search features beyond what exists.
- An in-TUI switcher for GitHub hosts or repos.
- Windows-specific polish (macOS and Linux first).

## Anti-goals

- Becoming slow or noisy against the API: no per-keystroke requests, and
  polling must stay modest.
- Silently wrong graphs: if dependencies cannot be determined, show a flat
  list and say so, never a guessed graph.
- Writing into any repo: favorites and settings never touch the working tree.

## Decisions made with the owner

### Favorites
- A favorite identifies a workflow by **host, repo, and workflow file** (for
  example `github.com/acme/api · deploy.yml`). Favorites belong to the user,
  not to a repo, and apply in every repo.
- Stored in a personal config file outside any repo.
- Each favorite may have a user-chosen **alias**, shown in place of the file
  name. The alias is also accepted as a command in the `:` command bar.
- Inside a repo's workflows view, that repo's favorites are pinned at the top
  and marked with a star. A `:fav` view lists favorites from all repos in one
  table with alias, repo, and latest run status; from it the user can trigger
  a favorite or open its runs without switching repos first.
- Single-key toggle to add or remove a favorite, and a single key to set or
  edit the alias.

### Job graph
- Watching a run (pressing `enter` on a run) shows the jobs as a graph that
  **replaces** the current jobs list. A flat jobs list remains only as the
  fallback.
- Three drawing styles, switchable with a single key, **A is the default**:
  - **A**: left-to-right boxes with arrows; columns are dependency stages.
  - **B**: top-to-bottom tree where each job lists what it needs and shows its
    current step.
  - **C**: timeline lanes where bar length is duration and bars grow while
    the run is live.
- The graph updates in place every few seconds while the run is active.
- Dependencies come from the `needs:` entries in the workflow file at the
  run's commit, since the run data does not include them.
- Each matrix job is its own node, grouped under its job name.
- Arrow keys move between nodes; `enter` opens that job's log.

### Trigger
- `t` opens a form from the workflows view and from the favorites view.
- The form lists the workflow's declared inputs with their defaults, shows
  choices for dropdown inputs and toggles for boolean inputs, and includes a
  branch/ref picker.

### Enterprise
- Release 1 uses whatever host `gh` is logged in to, or the one in `GH_HOST`.
  The owner does not use Enterprise today and works on one host at a time.
  Enterprise behavior is expected to work but is untested.

## Constraints

- Language and UI stack stay as they are (Go, Bubble Tea, lipgloss).
- All GitHub access goes through the `gh` CLI.
- The binary must be named `gh-9s` and ship via the existing tagged-release
  workflow.

## Left to the builder (my calls, not the owner's)

- Config file location and format (suggest the standard per-user config
  directory, a small human-editable file).
- The exact keys for favorite toggle, alias edit, and style switch.
- Fall back from style A to style B automatically when the terminal is too
  narrow (suggested, not discussed in detail).
- How reusable-workflow jobs are drawn.
- How the run-finished notification is delivered (terminal bell, desktop
  notification, or both) and how often runs are polled.
- Log search behavior details (case sensitivity, highlight, next/previous).
- How to handle `needs:` when the workflow file is missing, unparseable, or
  differs from the run's commit.

## Calls made after the builder check (the designer's, open to change)

1. **Trigger form inputs.** `gh` has no command for them, so the form reads
   the workflow file from the chosen ref (default branch first, re-read when
   the user changes the ref) and parses `on.workflow_dispatch.inputs`. A
   workflow without `workflow_dispatch` cannot be triggered: `t` shows a
   message instead of the form. Input types the form cannot render
   (`environment`, `number`) become plain text fields.
2. **Finding the run after a trigger.** Record the time before dispatch, then
   poll for runs of that workflow with event `workflow_dispatch`, the chosen
   branch, and the current user, created after that time. Give up after about
   30 seconds, stay on the runs list, and show a message. Never open a run
   that does not match.
3. **Favorites commands.** Typing an alias in the `:` bar opens that
   favorite's runs. Aliases that collide with a built-in command or another
   alias are rejected when set. A favorite whose repo is inaccessible or whose
   file was renamed stays in the list, dimmed, marked unavailable. Latest-run
   status is fetched when `:fav` opens (one request per favorite, a few in
   parallel) and cached for about 30 seconds, with no continuous polling.
4. **Favorites in other repos or hosts.** Every `gh` call takes an explicit
   `-R [HOST/]OWNER/REPO`, so `:fav` works without switching. If `gh` is not
   logged in to that host, the favorite shows as unavailable.
5. **Graph scale.** Matrix jobs are collapsed into one node showing counts by
   status, and expand on `enter`. The graph scrolls in a viewport when it is
   larger than the screen.
6. **Testing.** Unit tests for the dependency and input parsers, and golden
   (fixture-based) tests for the three drawing styles.
