# Project views on the board (release 7)

## Why
A GitHub Project has saved **views** (Table, Board, Roadmap; "My items" with `assignee:@me`; …). The board shows
every item by Status only, so the user cannot get to what a view shows. The API lists each view's name, layout,
filter text, grouping and sorting (verified on a real Project: `views(first:20){nodes{name number layout filter
groupByFields verticalGroupByFields sortByFields}}`), but not the items of a view.

## Goals
- The board lists the Project's views and lets the user switch between them (second row of tabs under the
  existing Status tabs; keys `[` and `]` for the previous/next view; the view chosen last is kept per Project for the session).
- A view shows its items with its **filter** applied locally and its **sort** (single sort field, direction) when
  the field is one we read (Status, Title, Updated, Assignees, Labels); Status tabs keep working inside the view.
- Honesty: when a filter or sort cannot be applied exactly, the view still opens, shows everything it can, and
  says so in one line ("filter not fully applied: <term>"). Never a silently wrong list.
- Roadmap views are listed but marked "roadmap (not shown)"; opening one shows the All view with that note.

## Non-goals
Writing views, creating views, grouping rows by a field (the Status tabs stay the only grouping), custom fields
other than Status, date/iteration filters, the roadmap drawing.

## Filter terms supported (everything else is reported as not applied)
`assignee:LOGIN|@me|none`, `label:NAME`, `-label:NAME`, `status:NAME`, `-status:NAME`, `is:open|closed|merged`,
`is:issue|pr|draft`, `repo:OWNER/NAME`, `no:assignee|label|status`, bare words (title contains), terms joined by space = AND,
comma-separated values = OR, quoted values. `@me` is the logged-in user of the Project's host
(`viewer{login}`, one extra query, cached for the session).

## Data
Views come with the existing Project query (same request: add `views(first:20){nodes{name number layout filter
sortByFields(first:1){nodes{field{... on ProjectV2FieldCommon{name}} direction}}}}`), so no extra call. Read-only
(no `mutation`). Names/filters pass through `CleanText`.

## Calls I made (change any)
- Local filtering of the 200 loaded items, not a server query.
- Views row appears only when the Project has more than the default views; hidden when `views` is empty.
- Unsupported terms are listed, not guessed.

## Review focus
Empty/huge filter strings, hostile view names, filter with unbalanced quotes, `@me` lookup failure, views > 20,
selected view deleted on refresh, narrow terminals (view row fits width), 200+ items with a filter (counts say `N+`).
