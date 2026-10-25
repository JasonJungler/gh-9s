---
title: Settings
description: Every setting, its default, and how to change it.
---

Open the settings screen with `:settings`, or change one value from the command bar with `:set key value`. The command bar completes both the keys and the values.

## The settings screen

The screen lists every setting with its current value and where the value comes from:

- `file`: set in `settings.json`
- `env`: set by an environment variable
- blank: the default

| Key | Action |
|---|---|
| `j` / `k` | Move |
| `enter` or `→` | Next value |
| `←` | Previous value |

Every change is saved to `settings.json` immediately.

If an environment variable overrides a setting, the screen says when your change has no effect.

## Settings

| Key | Default | Values | Meaning |
|---|---|---|---|
| `icons` | `ask` | `ask`, `nerd`, `plain` | `ask` asks on the next start. `nerd` uses Nerd Font icons. `plain` uses ordinary symbols. Env: `GH9S_ICONS`. |
| `preview` | `true` | on / off | Show the selected item in a pane on the right of the lists |
| `preview_width` | `40` | 25 to 60 | Percent of the screen the preview pane takes |
| `preview_min_columns` | `120` | 80 to 400 | The narrowest terminal that shows the preview pane |
| `mouse` | `true` | on / off | Scroll with the mouse wheel. See [Interface basics](/gh-9s/guides/interface/#mouse). |
| `delete_branch_after_merge` | `false` | on / off | Starting value of the "delete branch?" checkbox in the merge dialog |

Examples:

```
:set preview_width 35
:set preview off
:set icons nerd
```

## When the file is unreadable

- Unknown keys in `settings.json` are kept.
- A file that is not valid JSON is never overwritten. gh-9s starts with the defaults and says so. Changes last only for the session.
