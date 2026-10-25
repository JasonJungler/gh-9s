---
title: Repositories
description: Switch between repositories and pin the ones you use often.
---

gh-9s always works on one repository at a time. You can change it at any moment.

## Switching with a command

- `:repo OWNER/REPO` switches every view to that repository. For another host, use `HOST/OWNER/REPO`.
- gh-9s checks that the repository exists before it switches.
- The PR of the current branch (`P` or `:cpr`) only exists in the repository of your current directory.

## The repository picker

`:repo` on its own opens the picker.

- Type to search.
- `tab` switches between two lists:
  - **My access**: your own repositories and those of your organizations.
  - **Global**: all of GitHub. It needs at least two characters and waits a moment after you stop typing.
- `enter` switches to the selected repository.
- The first row is the repository of your current directory.
- With nothing typed, the repositories you used recently are listed.
- Pinned repositories carry a ★. `ctrl+f` pins or unpins the selected one.

## Pinning repositories

Pinned repositories appear in the Repos tab of `:fav`.

- `f` pins or unpins the repository of the current runs, pulls or issues list.
- `:fav repo [OWNER/NAME]` pins the current repository, or the one you name.

In the Repos tab of `:fav`:

| Key | Action |
|---|---|
| `enter` | Switch to the repository |
| `a` | Pin a repository |
| `f` or `d` | Unpin (asks first) |
| `o` | Open in the browser |

Pinned repositories and Projects are stored in `pins.json`. See [Files and environment](/gh-9s/reference/files-and-env/).
