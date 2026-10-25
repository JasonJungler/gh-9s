---
title: Commands
description: Every command you can type in the command bar.
---

Type `:` to open the command bar. Commands can be typed in full or with any of the short forms listed below.

| Command | Short forms | Does |
|---|---|---|
| `:runs` | `:run`, `:r` | Workflow runs |
| `:workflows` | `:wf`, `:w` | Workflows |
| `:pulls` | `:pr`, `:prs` | Open pull requests |
| `:issues` | `:is` | Issues |
| `:inbox` | `:in` | The inbox |
| `:cpr` | `:checks` | Checks of the current branch's PR |
| `:fav` | `:favs`, `:favorites` | Favorites (Workflows, Repos and Projects tabs) |
| `:fav repo [OWNER/NAME]` | | Pin the current repository, or the one you name |
| `:repo` | | Open the repository picker |
| `:repo OWNER/REPO` | | Switch to another repository (`HOST/OWNER/REPO` for another host) |
| `:settings` | | Open the settings screen |
| `:set KEY VALUE` | | Change one setting. See [Settings](/gh-9s/guides/settings/). |
| `:icons` | | Ask again whether your terminal shows Nerd Font icons |
| `:<alias>` | | Open the runs of the favorite workflow with that alias |
| `:q` | `:quit`, `:exit` | Quit |

Aliases cannot have the name of a built-in command. Commands complete from the built-ins, your aliases and (after `:repo `) your recent and cached repositories.
