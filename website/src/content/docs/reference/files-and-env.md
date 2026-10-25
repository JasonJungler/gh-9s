---
title: Files and environment
description: Where gh-9s stores its files and which environment variables it reads.
---

## Files

gh-9s keeps its own data in your personal config directory. It never writes into a repository.

| File | Contents | Location |
|---|---|---|
| `settings.json` | Your settings | The gh-9s config directory. Override with `GH9S_SETTINGS`. |
| `favorites.json` | Favorite workflows and their aliases | macOS: `~/Library/Application Support/gh-9s/`. Linux: `~/.config/gh-9s/`. |
| `pins.json` | Pinned repositories and Projects | Next to `favorites.json`. Override with `GH9S_PINS`. |
| `drafts.json` | Unsent inline comments, per pull request | Next to `favorites.json` |

If a file cannot be read, gh-9s says so. It never overwrites a file it could not read:

- A broken `favorites.json` disables favorites for the session.
- A broken `pins.json` makes pins last for the session only.
- A broken `settings.json` makes settings last for the session only.

## Environment variables

| Variable | Effect |
|---|---|
| `GH9S_ICONS` | `nerd` or `plain`. Overrides the `icons` setting. |
| `GH9S_IMAGES` | `auto`, `iterm`, `sixel`, `blocks` or `off`. See [Images and Markdown](/gh-9s/guides/images-and-markdown/). |
| `GH9S_SETTINGS` | Path of another settings file |
| `GH9S_PINS` | Path of another pins file |
| `BROWSER` | Browser used by `o`. Defaults to the system browser. Under WSL, the Windows browser is used. |
| `EDITOR` | Editor used by `ctrl+e` for long text |
| `GH_HOST` | GitHub host, if not github.com (also followed by `gh` itself) |

Environment variables override the settings file. The settings screen shows which setting an environment variable is controlling.

## Authentication

gh-9s uses the `gh` CLI for all GitHub access. Authentication, proxies and GitHub Enterprise follow `gh`'s configuration. If something does not work, first check `gh auth status`.
