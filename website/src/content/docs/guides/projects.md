---
title: Project boards
description: View GitHub Projects (v2) as boards and change cards safely.
---

gh-9s can show a GitHub Project (v2) as a board, and can make a few changes to its cards.

## Pin a Project

Open the favorites view with `:fav`, go to the Projects tab, and press `a`.

- Pick a Project from the list of your Projects and your organizations' Projects, or
- Type `[HOST/]OWNER/NUMBER`.

Press `enter` on a pinned Project to open its board.

## The board

The board has one tab per Status column, in the Project's order:

- `All` comes first.
- `No status` appears when some card has no status.

It shows the first 200 cards.

| Key | Action |
|---|---|
| `tab` / `l` / `→` | Next column (Status) |
| `shift+tab` / `h` / `←` | Previous column |
| `j` / `k` | Move between cards |
| `enter` | Open the issue or PR in its own repository (Enterprise hosts too) |
| `/` | Filter |
| `R` | Refresh (at most once in a while) |
| `o` | Open in the browser. A draft card opens the Project. |
| `f` | Pin or unpin this Project |

Press `enter` on a card to see its details in the preview pane, the same as in the lists.

## Saved views

The Project's saved views appear in a second row. Switch between them with `]` and `[`.

gh-9s applies each view's filter and first sort to the loaded cards. The filters it understands are:

- `assignee:@me`
- `label:`
- `status:`
- `is:open`, `is:pr`, `is:issue`, `is:draft`, `is:merged`
- `repo:`
- `no:`
- Plain words, and `-` to exclude

Sorts are Title, Status and Updated. Anything else is named in a note instead of guessed. Roadmap views are not drawn.

## Column board

Press `b` to switch to the column board. Every Status is shown as a column, side by side.

- `h` and `l` change column. `j` and `k` change card.
- Each column is at least 24 columns wide. The board needs a terminal at least 60 columns wide.
- The preview pane is not shown in this layout.

## Changing cards

Every change asks a question that says exactly what will change, and nothing changes before you confirm.

| Key | Action |
|---|---|
| `s` | Pick a new Status for the card |
| `H` / `L` | In the column board, move the card to the previous / next column |
| `a` | Change the assignees of an issue or PR card, in the card's own repository |
| `n` | Add an issue or PR. Enter it as `OWNER/REPO#N` or as its address on the Project's host. It gets the Status of the column or tab you are in. |

Only one change runs at a time. After a change, the board is read again.

Removing cards, editing drafts and changing other fields are not available from gh-9s. Use the browser for those.

## Permissions

Reading a Project needs the `read:project` permission. Changing one needs the `project` permission. Add it with:

```sh
gh auth refresh -s project
```

For GitHub Enterprise, add `--hostname HOST`.
