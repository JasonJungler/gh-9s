# Project writes (release 10)

## Why
The board is read-only. The user wants to change what a card says from the terminal: its Status, who is assigned, and
adding items to a Project. First time gh-9s changes a Project, so every write asks first and says what it did.

## Goals
- **Move a card to another Status**: `s` on a card opens a choice dialog with the Project's Status options (current one
  marked); picking one asks "Move <title> to <Status>?" and then runs `updateProjectV2ItemFieldValue`. In the columns
  layout `H`/`L` move the selected card to the previous/next column in one step (same confirm).
- **Assign people**: `a` on an issue or PR card opens the same assignee dialog the issue/PR screens use (the existing
  `gh issue edit` / `gh pr edit` machinery, in the card's own repo and host). Not offered for drafts.
- **Add an item**: `n` asks for an issue or PR (`OWNER/REPO#N`, or a github URL on the Project's host); it is looked up
  first (title shown in the confirm: "Add <title> to <Project>, Status <current column>?"), added with `addProjectV2ItemById`,
  then its Status is set to the column the cursor is in (when that column is a real Status).
- After a write the board refreshes at once and keeps the selection by id; success and failure are on the status line.
- Needs the `project` token scope. Without it the write fails with: "Changing a Project needs the project permission.
  Run: gh auth refresh -s project" (+ `--hostname HOST` for Enterprise). Detected from the error text only for the
  scope message; any other error is shown as is.

## Non-goals
Removing or archiving items, creating draft cards, creating/editing Projects or views, fields other than Status,
reordering cards inside a column, bulk moves, undo.

## Constraints
- Every write confirms with the exact change in words; `esc` cancels; nothing is sent before `y`/enter on the confirm.
- Identity captured at key-press time (Project, item id, option id); an answer for another Project or after leaving
  the screen is dropped; a second write while one is in flight is refused ("still working").
- GraphQL request bodies go over stdin as JSON (`gh api graphql --input -`), ids are never pasted into the query text.
- Read-only tests that grep for `mutation` change: the grep now allows exactly the three mutations above, only in
  `internal/ghcli/projectwrite.go`.
- Status option ids, field id, project id and item ids come with the existing read query (extra fields only).

## Calls I made (change any)
- `s` for status, `a` for assignees, `n` for new item, `H`/`L` for moving between columns.
- Moving to "No status" is not offered (GitHub needs a clear-field mutation; left out for now).
- Optimistic update is not done: the board shows the change after the refresh.

## Review focus
Scope-missing path, item deleted meanwhile, option removed meanwhile, network failure mid-write (status line says
unknown outcome, refresh shows truth), double key press, Enterprise host, an issue in a repo the user cannot edit,
hostile titles in confirm text, stale dialog after the screen changes.
