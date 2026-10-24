# gh-9s release 6 — favorites in tabs: workflows, repos, GitHub Projects

## Intent

Today `:fav` lists favorite workflows only. The user works across several repos and GitHub Projects (v2) boards; they want one place to reach all three: a favorites screen with tabs — **Workflows**, **Repos**, **Projects** — where each tab lists what was pinned and `enter` goes there. A pinned Project opens as a board of its issues and pull requests grouped by status, which they can browse and open. **This release is read-only**: nothing on a board is changed from gh-9s (changing status, assigning, adding items is a later release).

## Goals

- `:fav` (and the favorites keys that exist) opens a tabbed favorites screen; the first tab is today's workflow list, unchanged.
- Pin and unpin repos; pin and unpin Projects.
- Open a Project as a board: tabs by Status with counts, items listed (issue / pull request / draft), open an item in the PR or issue screen of its own repo (any host), `o` for the browser.
- Works with GitHub Enterprise hosts, like the rest of gh-9s.

## Non-goals / anti-goals

- No writing to Projects: no status change, assign, add, remove, archive. (Not even by accident: no key on the board may call a mutating command.)
- No re-implementation of the Project's saved views, filters, sort, grouping by other fields, iterations, roadmap/table/board layouts. One grouping: the Status field.
- No `admin:org` or write scopes requested; only `read:project`.
- Existing `favorites.json` (an array of workflow favorites) keeps its format and meaning so older gh-9s versions still read it; the new pins go in their own file.
- Not a failure: Projects that need a token scope the user lacks. gh-9s says exactly what to run and keeps working.

## What the user sees

### Favorites screen
- Tab bar `Workflows | Repos | Projects` with counts, like the inbox: `tab`/`shift+tab`/`l`/`h` switch tabs; the footer and `?` help list each tab's keys. `:fav` opens it on the last tab used (default Workflows).
- **Workflows tab**: exactly today's favorites screen and keys (enter runs, t trigger, f unfavorite, a alias).
- **Repos tab**: rows `HOST/OWNER/NAME` (host omitted for github.com), with the repo's description and when it was last pushed if cheap to get (builder's call; drop it if it costs a request per row). Keys: `enter` switches to that repo (same effect as `:repo OWNER/NAME`, landing where `:repo` lands), `a` pins a repo (prompt for `[HOST/]OWNER/NAME`, validated as `:repo` validates), `f` or `d` unpins the selected row (asks first), `o` opens it in the browser. Also `:fav repo` pins the current repo (the one shown in the title) and the repo picker (`:repo`) marks pinned repos and lets `ctrl+f` toggle the pin on its selected row.
- **Projects tab**: rows: title, `OWNER/#NUMBER`, host if not github.com, open/closed. Keys: `enter` opens the board, `a` pins a Project (prompt for `OWNER/NUMBER`, e.g. `acme/3`, or `HOST/OWNER/NUMBER`; checked by reading the Project, its title is stored), `f` or `d` unpins (asks first), `o` opens it in the browser.
- Pins are kept in order of pinning; newest at the bottom. A pin whose repo or Project is gone shows a dimmed row with the error and can be unpinned.

### The board screen (a pinned Project opened)
- Title line: `<project title>  OWNER/#N`. A tab bar with one tab per Status value in the Project's own order, plus `No status` if some items have none; each tab shows its count. `tab`/`shift+tab`/`h`/`l` switch tabs; all items are also reachable through the first tab `All`. (My call: `All` first, then the statuses.)
- Rows in a tab: type icon (issue open/closed, PR open/draft/merged/closed, draft item), `REPO #N`, title, assignees, labels (compact), updated. Sorted as the Project returns them (its own order).
- `enter` opens the item in the existing PR or issue screen of its own repo and host (like inbox rows); a draft item (no repo) has nothing to open and says so. `o` opens the item in the browser (a draft item opens the Project). `/` filters the tab, `R` refreshes (not more often than every 5 s), `esc` back. The preview pane (release 5d) works on board rows that are issues or PRs, exactly as in the inbox.
- Loads at most the first 200 items (two pages of 100, one after the other); if the Project has more (`totalCount`), the screen says "N more in the browser" and the tab counts are shown as `N+`.
- The board is not polled; it refreshes on `R` and when re-entered after more than a minute.

### Errors
- Missing token scope (`gh` says `missing required scopes [read:project]`): the Projects tab and the board show: `Projects need the read:project permission. Run: gh auth refresh -s read:project` (with `--hostname HOST` for Enterprise hosts). No other part of gh-9s is affected.
- Not found / no access / network: the error text (sanitised), the pin stays, `R` retries. GitHub 502/503/504 are retried once automatically (the helper from the PR list fix).

## Constraints

- Go, Bubble Tea v1, everything through the `gh` CLI (only `internal/ghcli` runs it; calls to a host go with `--hostname`/`--owner` correctly; use the timeouts helpers). Project data comes from one read-only GraphQL query per page through `gh api graphql` (as the inbox does): `repositoryOwner(login:){... on ProjectV2Owner{projectV2(number:){title closed field(name:"Status"){... on ProjectV2SingleSelectField{options{name}}} items(first:100, after:){totalCount pageInfo{hasNextPage endCursor} nodes{fieldValueByName(name:"Status"){... on ProjectV2ItemFieldSingleSelectValue{name}} content{__typename ... on Issue{number title state url updatedAt repository{nameWithOwner} assignees(first:5){nodes{login}} labels(first:5){nodes{name}}} ... on PullRequest{number title state isDraft merged url updatedAt repository{nameWithOwner} assignees(first:5){nodes{login}} labels(first:5){nodes{name}}} ... on DraftIssue{title}}}}}}}`. One query gives the status order, each item's status, state and update time, so there is no second request. `gh project ...` commands are not used.
- Remote text is sanitised (`CleanText`); names from the pins file are validated like favorites are (no leading `-`, plain tokens) before they reach a command line; a hand-edited or broken `pins.json` is never overwritten, the screen says so and the pins stay in memory only for the session (same rule as settings and favorites).
- Pins file: `pins.json` next to `favorites.json`, atomic write, mode 0600, re-read before write, shape `{"repos":[{"host","repo"}],"projects":[{"host","owner","number","title"}]}`; `GH9S_PINS` overrides the path (for tests); no test touches the real config.
- All existing keys keep working; the Workflows tab behaves exactly as the old screen (its tests stay).
- Safety principles of the project hold: stale async results are dropped by identity (project, tab, request), an action uses the identity captured when the key was pressed, no write operation exists.
- TDD, no network in tests (seams), one fresh-reviewer pass at the end, finishing menu; no push/tag without the user's approval. README updated.

## Calls I made while writing this (from the builder check; say if you disagree)

- Item state, draft/merged flags and updated time come with the same GraphQL query (above), not from `gh project item-list`, which does not return them. Status order is read in that query too; no second request.
- The screen has partial counts when the Project is bigger than 200 items: tab counts read `N+` and the footer says how many more exist.
- `:fav` and the existing entry points all open the tabbed screen; the Workflows tab keeps every old key; aliases (`:ALIAS`) work as before.
- The last tab used is remembered for the session only (not saved).
- `enter` on a pinned repo does what `:repo OWNER/NAME` does (switches the repo and resets the screen stack the same way), so `esc` behaves as it does after `:repo`.

## Left to the builder

Icons and colours for the new tab bar and item types (use the icon table; plain + Nerd sets), key choices not named above, the column layout of rows, how the GraphQL result is mapped to rows, whether repo metadata is fetched for the Repos tab, the wording of messages.
