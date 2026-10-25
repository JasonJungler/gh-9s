---
title: Inbox
description: See what needs you across all repositories and hosts.
---

The inbox (`0` or `:inbox`) shows what needs your attention, across every repository and every host that `gh` is logged in to.

## Tabs

| Tab | Shows |
|---|---|
| **Assigned issues** | Issues assigned to you. This tab opens first. |
| **Review requested** | PRs waiting for your review |
| **My PRs** | Your PRs, with checks and review state |
| **Mentions** | Places where you were mentioned |

Switch tabs with `tab` or `l` (next) and `shift+tab` or `h` (previous).

## Keys

| Key | Action |
|---|---|
| `j` / `k` | Move |
| `enter` | Open the item in the PR or issue screen. The review screen and all actions are available. |
| `esc` | Return to the same tab and row |
| `o` | Open in the browser |
| `/` | Filter the items that are loaded |
| `R` | Refresh now (at most once every few seconds) |

## Refresh and new items

The inbox refreshes itself about once a minute. Rows that appeared since your last refresh are marked `●` until you leave that tab.

If one host cannot be reached, the inbox shows a note for it while the other hosts still list their items.
