# gh-9s: finishing the Project features

Status: draft, waiting for approval. Nothing is built.

## Intent

The Project board can read, filter, show columns, change Status, change assignees and add an item. The user asked
for everything listed as "not done" for Projects to be added, so the board can replace the browser for day-to-day
Project work. This is one design with three releases, in this order, each reviewed and published only on the
user's explicit "發佈".

## Releases

### A. Reading more of a Project (read-only, no new permission)

1. **More than 200 cards.** `FetchProject` keeps reading pages until the Project is read (cap 2000 cards, 20 pages
   of 100). The footer says "first N of M" only when the cap is hit. A failure on a later page keeps what was
   loaded and says so in the note (today the whole board is discarded).
2. **More than 20 views.** Views are read with a cursor too (cap 100); a cut-off is named in the note.
3. **Sort by Assignees / Labels** (first value, case-insensitive, empty last) and the direction of every sort.
4. **Roadmap views** are drawn as a list ordered by their date field, grouped by month ("2026-11"), cards without a
   date last. No bars; my call: terminal width makes a real timeline poor.
5. **Cursor after coming back** from an issue or PR opened off the board: the board keeps its selection by card id
   (the same mechanism used after a refresh).
6. **`R` during a load** does nothing but say "already loading".
7. **Scope hint** appears only when the error is about the `read:project` or `project` scope.
8. **Short terminals:** below 8 usable lines the columns layout falls back to the list with a note.

### B. Writes on cards (needs the `project` scope, same rules as release 10)

Every write asks first with the exact change in words, uses identity captured at key-press time, is refused while
another write runs, refreshes the board afterwards and keeps the selection by id. Mutations live only in
`internal/ghcli/projectwrite.go`.

1. **`x` remove the card from the Project** (`deleteProjectV2Item`). The issue/PR itself is untouched. A draft card is
   deleted for good, so its confirm says so ("This draft exists only in the Project and cannot be restored").
2. **`X` archive the card** (`archiveProjectV2Item`). Archived cards are not read by the board, so un-archiving stays
   in the browser; the confirm says so.
3. **`N` create a draft card** (`addProjectV2DraftIssue`): a text dialog for the title; the body is edited later; the
   card gets the Status of the current column/tab like `n`.
4. **`e` edit a draft's title/body** (`updateProjectV2DraftIssue`) using the existing text dialog and Ctrl+E editor.
5. **Other fields** with `v` (value): lists the Project's fields of type single select, text, number, date and
   iteration (other types are not offered). Single select and iteration: choice dialog. Text/number/date: text dialog
   (date `YYYY-MM-DD`, number validated, empty clears). Mutations `updateProjectV2ItemFieldValue` /
   `clearProjectV2ItemFieldValue`. The read query learns the field list (`fields(first:50)`) and each card's values;
   the preview pane shows them, the columns card does not (width). A field with more than 50 options is read only
   partly and the dialog says so.
6. **Reorder inside a column** with `ctrl+up` / `ctrl+down` in the list and columns layouts
   (`updateProjectV2ItemPosition` with `afterId`; empty = top). Only offered when the board shows the Project's own
   order (no sort applied); otherwise the status line says why.
7. **Bulk:** `space` marks cards (shown with `●`); `s`, `x`, `X`, `a` act on all marked cards after one confirm that
   says how many ("Move 4 cards to Done?") and names the first three. Writes run one after another; the first
   failure stops the run and reports how many were done. Marks are cleared by `esc`, a view switch, and a refresh
   drops marks of cards that are gone. Cap 50 marked cards.
8. **Undo `u`** of the last write of this session on this Project, only where an exact inverse exists: Status/field
   change (previous value remembered at key-press time), add item (remove it), draft create (delete it, with
   confirm). Remove and archive say "cannot be undone here". One level; it asks like any write. If the card's value
   is no longer what the write set (someone else changed it) the status line says "changed since; not undone".

### C. From an issue or PR to a Project (needs the `project` scope)

On the issue screen, issue list, PR list and PR detail: `P` lists the Projects the user pinned plus the Projects of
the item's owner (reusing `ListProjects`) in a multi-select with the ones the item is already on checked (read from
`projectItems`, as `ResolveContent` does). Confirming adds/removes after one question that names each change ("Add to
Roadmap, remove from Sprint 4"). Add uses `addProjectV2ItemById`; remove looks up the item id on that Project and uses
`deleteProjectV2Item`. Status is not set; the status line says "added; set its Status from the board".

## Goals / non-goals / anti-goals

Goals: everything above, with the codebase's safety rules (identity at key press, stale answers dropped, JSON over
stdin, remote text cleaned, token only to the repo host, confirm in words).
Non-goals: creating/editing Projects themselves, creating/renaming fields, editing views, workflows, Insights,
un-archiving, mouse drag.
Anti-goals: a write without a confirm; a bulk run that continues after a failure; an undo that overwrites someone
else's newer change; guessing a field type.

## Constraints

Go 1.27 via mise (`mise exec -- go ...`), existing libraries only, only `internal/ghcli` runs `gh`. New keys must not
collide with the board's existing ones (`s a n H L b f o / R [ ] enter h l j k g G tab J K`); reorder therefore uses
`ctrl+up`/`ctrl+down`. Keys appear in `?` help, the footer and the README. Tests use fakes at the seams, with mutation
checks on every guard; one fresh Opus review per release; the "Projects are only written in the write files" test is
extended to any new write file.

## Calls I made (please check)

- Three releases, A then B then C; B is the biggest and may split into B1 (remove, archive, drafts, fields) and B2
  (reorder, bulk, undo) with a release in between.
- Roadmap is a dated list, not bars. Archived cards stay invisible and are not un-archived.
- Card cap 2000; marked-card cap 50; undo is one level, session only.
- Reorder uses ctrl+up/ctrl+down. `P` on issues/PRs adds without setting Status.
- I skipped the separate builder-check subagent for this draft; the user can ask for it.

## Manual checks I cannot run

Real Enterprise host paging; archive/delete on a throwaway Project; date/number/iteration fields on a Project that has
them; reorder showing correctly in github.com's web UI; the `project` scope error text on Enterprise.
