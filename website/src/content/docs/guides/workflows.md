---
title: Workflows and favorites
description: List workflows, trigger them, enable or disable them, and keep favorites with aliases.
---

The workflows view (`4` or `:wf`) lists the workflows of the current repository. Favorites are pinned to the top with a ★.

## Workflow keys

| Key | Action |
|---|---|
| `enter` | Show the runs of the workflow |
| `t` | Trigger the workflow (opens the [trigger form](#trigger-a-workflow)) |
| `f` | Favorite or unfavorite |
| `a` | Set an alias |
| `e` | Enable or disable the workflow |
| `o` | Open in the browser |

## Trigger a workflow

Press `t` on a workflow. The form is built from the workflow's `workflow_dispatch` inputs:

- Text fields for free text.
- Dropdowns, changed with `←` and `→`.
- Booleans, toggled with `space`.
- A branch picker for the branch to run on.

Required inputs block submission until they are filled in.

After you submit, gh-9s finds the run that was created and opens its [job graph](/gh-9s/guides/job-graph/). If it cannot identify the run within 30 seconds, it tells you and leaves you on the list. The workflow has still been dispatched.

## Favorites and aliases

A favorite is one workflow in one host and repository, for example `github.com/acme/api · deploy.yml`.

- Press `f` to favorite a workflow.
- Press `a` to give it an alias, then type `:<alias>` from anywhere to open its runs.
- Aliases cannot shadow built-in commands. If you pick a name such as `runs`, gh-9s refuses it.

Favorites are stored in your personal config directory, never inside a repository. See [Files and environment](/gh-9s/reference/files-and-env/) for the exact path.

If the favorites file cannot be read, favorites are disabled for the session and the file is left untouched.

The `:fav` view groups favorites into tabs. See [Repositories](/gh-9s/guides/repos/) for the Repos tab and [Projects](/gh-9s/guides/projects/) for the Projects tab.
