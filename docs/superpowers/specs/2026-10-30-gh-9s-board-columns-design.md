# Board columns (release 8)

## Why
The board shows one Status at a time. GitHub's web board shows every Status side by side; the user wants that in the
terminal, read-only.

## Goals
- `b` toggles the Project board between **list** (today, default) and **columns** (one column per Status tab of the
  current view: the Status options in order, extra statuses, then "No status" when needed). The choice is kept for the session.
- Each column: header `Name (count)` (`N+` when truncated), then cards. A card is 3 lines: `icon REPO #N`, the title
  (wrapped to the column width, at most 2 lines, `…` when cut), `assignees · labels` (one line, cut). Drafts show
  "(draft)"; "(no access)" cards show as such. Cards are separated by one blank line; the selected card is highlighted.
- Keys in columns: `h`/`l`/`←`/`→` change column, `j`/`k`/`↓`/`↑` change card (keeping the row index when possible,
  else the nearest), `g`/`G` first/last card of the column, `enter` open (as today), `o`, `f`, `R`, `/` filter,
  `[`/`]` change view, `tab`/`shift+tab` also change column. Each column scrolls vertically to keep the selected card visible
  and remembers its scroll for the session of the screen.
- Width: every column is at least 24 columns wide. If all columns do not fit, show a window of columns around the
  selected one with `‹`/`›` markers at the edges; the header row never exceeds the terminal width.
- Under 60 columns of width the columns layout is refused with a one-line status ("columns need at least 60 columns")
  and the list stays.
- The preview pane is off while in columns (the width is needed); `p` still toggles it for the session, and then
  the columns window uses the remaining width. Footer and `?` help list the new keys.
- The selected card is the same item as in the list: the list's selection and the columns' selection stay in sync when
  toggling `b`.
- Filter `/`, view filters and sort apply exactly as in the list; counts agree with the list tabs.

## Non-goals
Dragging or moving cards, editing, grouping by fields other than Status, swimlanes, mouse clicks on cards
(the wheel scrolls the selected column), colours per status.

## Calls I made (change any)
- 3-line cards, wrap at most 2 title lines.
- The columns layout is a second way to draw the same data; the list stays the default and all list tests keep passing.
- Wide characters (CJK) are measured by display width, and long words are cut with `…`, never wrapped across columns.
- Selected-card memory per column is not kept across a refresh (the card is found again by its id).

## Review focus
No columns / one column / 40 columns; 200+ items with N+ counts; CJK and emoji in titles; terminal 60 to 300 wide;
selection when the selected card disappears on refresh or filter; empty columns (selection cannot stay on them: `h`/`l`
still enters an empty column, `j`/`k` do nothing); switching view while in columns; wheel scrolling; window markers.

## As built (deviations from the text above)
- A card is four lines (item, title on up to two lines, who and what) plus a blank one, so a column scrolls in steps of 5 rows.
- The preview pane is not shown in columns; `p` there says so. (The pane would take the width the columns need.)
- Long words in a title are wrapped across lines (never cut with `…`); only a third title line is cut.
- `h`/`l` land on the card last selected in that column, not on the same row index.
- Column headers show the count before the typed `/` filter, as the list tabs do.
