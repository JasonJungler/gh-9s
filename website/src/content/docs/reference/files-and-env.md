---
title: Files and environment
description: Where gh-9s stores its files and which environment variables it reads.
---

## Files

gh-9s keeps its own data in your personal config directory. It never writes into the repository you are working in.

| File | Contents | Location |
|---|---|---|
| `settings.json` | Your settings | The gh-9s config directory. Override with `GH9S_SETTINGS`. |
| `favorites.json` | Favorite workflows and their aliases | macOS: `~/Library/Application Support/gh-9s/`. Linux: `~/.config/gh-9s/`. |
| `pins.json` | Pinned repositories and Projects | Next to `favorites.json`. Override with `GH9S_PINS`. |
| `drafts.json` | Unsent inline comments, per pull request | Next to `favorites.json` |
| `launch.json` | The model last used for each coding agent, for [sending an issue to an agent](/gh-9s/guides/send-to-agent/) | Next to `favorites.json` |
| `wiki/` | A copy of each wiki you opened, kept so that it loads fast. It is only a cache: gh-9s throws it away and clones it again when it is damaged | `gh-9s/wiki/HOST/OWNER/REPO.wiki` under your cache directory (macOS: `~/Library/Caches`, Linux: `~/.cache`) |
| `wiki/lost/` | Text you wrote for a wiki page that could not be saved (the page changed on GitHub, or GitHub refused the push). The message on screen names the file | Under the same `wiki/` directory. Delete the files whenever you like. |

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
| `VISUAL`, `EDITOR` | Editor used by `ctrl+e` for long text and for editing wiki pages (`VISUAL` first, else `EDITOR`, else `vi`) |
| `GH_HOST` | GitHub host, if not github.com (also followed by `gh` itself) |

Environment variables override the settings file. The settings screen shows which setting an environment variable is controlling.

## Authentication

gh-9s uses the `gh` CLI for all GitHub access. Authentication, proxies and GitHub Enterprise follow `gh`'s configuration. If something does not work, first check `gh auth status`.
