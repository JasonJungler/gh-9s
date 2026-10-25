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

It shows up to 2000 cards, read 100 at a time. If a later page cannot be read, the cards read so far are kept and a note says so.

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

Sorts are Title, Status, Assignees, Labels and Updated. Anything else is named in a note instead of guessed. A roadmap view is listed by its first date field (cards without a date last), with that date in the last column; there are no bars.

## Column board

Press `b` to switch to the column board. Every Status is shown as a column, side by side.

- `h` and `l` change column. `j` and `k` change card.
- Each column is at least 24 columns wide. The board needs a terminal at least 60 columns wide and 8 lines of room; otherwise the list is shown.
- The preview pane is not shown in this layout.

## Changing cards

Every change asks a question that says exactly what will change, and nothing changes before you confirm.

| Key | Action |
|---|---|
| `s` | Pick a new Status for the card |
| `H` / `L` | In the column board, move the card to the previous / next column |
| `a` | Change the assignees of an issue or PR card, in the card's own repository |
| `n` | Add an issue or PR. Enter it as `OWNER/REPO#N` or as its address on the Project's host. It gets the Status of the column or tab you are in. |

| `x` | Remove the card from the Project. An issue or PR stays where it is; a draft is gone for good, and the question says so. |
| `X` | Archive the card. Archived cards are not shown here; un-archive them in the browser. |
| `N` | Create a draft card, with the Status of the column you are in |
| `e` | Edit a draft's title and text |
| `v` | Change another field: single select and iteration from a list; text, number (`1.5`) and date (`YYYY-MM-DD`) typed; `-` clears it |
| `ctrl+↑` / `ctrl+↓` | Move the card one place in its list or column (not in a view with its own sort) |
| `u` | Undo the last Status or field change, added card or new draft of this session |

Only one change runs at a time. After a change, the board is read again.

### Several cards at once

`space` marks the card under the cursor (shown as `●`, at most 50). Then `s`, `a`, `x`, `X` and `H` / `L` act on all marked cards on screen after one question that names the first three. The cards are changed one after the other and the run stops at the first failure, saying how many were done. `a` only adds people, and the cards must be in one repository. `esc` clears the marks.

### Undo

`u` takes back the last Status or field change, an added card or a new draft. It asks first. It cannot undo removals, archives, assignee changes, reordering or bulk changes. If someone else changed the card meanwhile it says "changed since" and does nothing.

### From an issue or PR

On the issue or pull request screen, and in the issue and PR lists, `M` lists your Projects and your organizations' with the ones the item is in ticked. Tick or untick, confirm the change in words, and the item is added to or taken out of those Projects. A newly added card has no Status; set it from the board.

## Permissions

Reading a Project needs the `read:project` permission. Changing one needs the `project` permission. Add it with:

```sh
gh auth refresh -s project
```

For GitHub Enterprise, add `--hostname HOST`.
