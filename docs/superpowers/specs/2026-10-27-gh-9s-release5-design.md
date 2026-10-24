# gh-9s release 5 — settings, Nerd Font icons, a preview pane

Three changes that build on each other, one version each, in this order: (1) a settings system and `:settings`, (2) Nerd Font icons with a first-run question, (3) a gh-dash-style layout with a preview pane on the right.

## Why

The user finds gh-dash's UI well designed and wants gh-9s to look and feel like it: icons from a Nerd Font, and a list with a preview pane on the right that follows the selection. Not every terminal has a Nerd Font, so the first start asks the user whether they can see the icons, and everything else keeps working without them. The user also wants a `:settings` screen, and wants the way settings are stored to leave room for configuration later. Success: someone opens gh-9s for the first time, answers one question, and gets a gh-dash-like screen; someone without the font sees the screen they see today (plus the preview pane if they want it).

## Non-goals / anti-goals

- No font detection by guessing: terminals do not report fonts, so the only source is asking.
- No change to what the plain (non-Nerd-Font) screens look like when icons are off, except that a preview pane may be there if the user has it on.
- No theme engine or colour customisation in this release (settings leave room for it; nothing is built for it).
- No new data sources: the preview shows what the existing detail fetches already give.
- A setting must never be able to make the screen unusable: unknown or invalid values fall back to defaults and are reported, never crash.

## Part 1 — settings

- A settings file next to the other personal files (`settings.json` in the gh-9s config directory). The existing `prefs.json` choice (delete branch after merge) moves into it; an existing `prefs.json` is read once, its value carried over, and left alone.
- Settings are declared in one registry: key, label, short description, kind (on/off, one of a list, whole number with a range), default, and how to validate. `:settings`, the file reader, the environment overrides and the tests all work from the registry, so a later release adds a setting by adding one entry.
- First settings: `icons` (`ask` until answered, then `nerd` or `plain`), `preview` (on/off, default on), `preview_width` (percent of the screen, 25–60, default 40), `preview_min_columns` (the narrowest terminal that shows the pane, default 120), `delete_branch_after_merge` (moved). Environment overrides: `GH9S_ICONS=nerd|plain` (wins over the file and over the question), and the file path can be changed with `GH9S_SETTINGS`.
- `:settings` opens a screen listing every setting with its current value and description; `j/k` move, `enter` or `space` or `←/→` change it (cycle, toggle, or step a number), the change applies at once and is saved at once. `esc` leaves. `:set key value` changes one setting from the command bar (with completion of keys and values), for people who prefer typing.
- A file that is not valid JSON is not overwritten: gh-9s starts with defaults, says so on the banner, and `:settings` shows the problem and does not save until the file is fixed or moved. Unknown keys in the file are kept when saving. Invalid values are replaced by the default in memory only and reported.

## Part 2 — Nerd Font icons and the first-run question

- An icon table with two sets, `nerd` and `plain`, addressed by meaning (PR open / merged / closed / draft, issue open / closed, check passed / failed / pending / skipped, review approved / changes requested / required, run status, branch, repo, comment, user, label, clock, tab icons for the inbox tabs, and so on). The plain set is exactly the symbols used today. All places that draw a symbol take it from the table.
- With `icons = nerd`: statuses, the PR/issue kind column, checks and review columns, the tab bars, the file list of the diff screen, and the graph node symbols use the Nerd Font icons with colours like gh-dash's (green open, purple merged, red closed/failed, yellow pending, grey draft).
- First run: when `icons` is `ask`, before the first screen, a full-screen question shows a row of sample glyphs (a few of the real icons) and asks "Can you see icons in the row above (not boxes or question marks)? y / n". `y` sets `nerd`, `n` sets `plain`, both are saved; `?` explains how to install a Nerd Font in a sentence and a link. If stdin is not a terminal or the question cannot be answered, nothing is asked and `plain` is used for this run without saving.
- `:settings` is how the choice is changed later, and a command `:icons` re-runs the question.
- Width: Nerd Font icons are treated as one column wide everywhere the layout counts columns; a layout test pins this so alignment does not drift. (Some terminals draw them two columns wide; the builder decides whether to add a setting for that. It must not break the plain set.)

## Part 3 — the preview pane

- Lists get a pane on the right that shows the selected row, like gh-dash: on the PR list, the issue list, and the inbox (every tab). The runs list and the others are not changed in this release.
- Layout: when the terminal is at least `preview_min_columns` wide and `preview` is on, the list takes the left part and the pane the right `preview_width` percent, separated by a border; below that width the list is full width as today. A key (the builder's choice) toggles the pane for the session; `:settings` changes it for good.
- PR pane: title, number, state/draft icon, author, `base ← head`, labels, review decision, checks (counts, and the failing ones by name), size (+/−, files), and the description rendered as markdown (the existing markdown renderer, including code colouring); below it the latest comments. Issue pane: title, state, author, labels, assignees, description, latest comments. Inbox rows use the same panes for their kind.
- The pane scrolls independently of the list (keys chosen by the builder, shown in the footer). It shows "loading…" while the detail is fetched and the error text if the fetch fails.
- The data comes from the same detail fetches the PR and issue screens use. Moving the selection fetches the new detail after a short pause (the builder picks the delay, around 250 ms), so holding `j` does not fire a request per row; results are cached for the session (a PR/issue is refetched when it changes updatedAt or after a few minutes); a result that arrives for a row that is no longer selected is kept in the cache but not shown.
- `enter` still opens the full screen, as before; the pane is only a preview.
- Looks, in both icon sets: rounded borders for the two panes, a tab bar with icons and counts on the inbox (the PR and issue lists keep their own title), selected row highlighted across the list width.

## Constraints

- Go, Bubble Tea v1, lipgloss; tests never touch the network or the user's real config (every file path comes from a seam).
- TDD, a fresh-reviewer pass per part, the finishing menu after each; no push/tag without the user's approval. README updated with each part.
- Existing keys keep working.

## Left to the builder

The exact icon code points per meaning (from the standard Nerd Font ranges), colours, the preview toggle key and scroll keys, the debounce delay and cache lifetime, the format of the settings screen, how `:set` completes, how the first-run question draws before the first screen exists.
