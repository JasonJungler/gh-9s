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
| `launch_target` | `auto` | `auto`, `orca`, `herdr`, `tmux` | Where `a` starts a coding agent for an issue. `auto` uses the environment gh-9s is running in. See [Send an issue to an agent](/gh-9s/guides/send-to-agent/). |
| `launch_agent` | `claude` | `claude`, `devin` | The coding agent to start. A launch remembers the one it used. |
| `launch_tmux_place` | `pane` | `pane`, `window` | Open the agent in a pane or a window. herdr uses this too (its window is a tab). |
| `launch_split` | `horizontal` | `horizontal`, `vertical` | Which way a new pane splits. herdr uses this too. |
| `launch_submit` | `true` | on / off | `on` sends the issue to the agent at once. `off` only puts it in the agent's input box and you press `Enter` there; Orca then uses its longer path. See [Paste only](/gh-9s/guides/send-to-agent/#paste-only-instead-of-submitting). |

Examples:

```
:set preview_width 35
:set preview off
:set icons nerd
```

## When the file is unreadable

- Unknown keys in `settings.json` are kept.
- A file that is not valid JSON is never overwritten. gh-9s starts with the defaults and says so. Changes last only for the session.
