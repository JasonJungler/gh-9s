---
title: Runs and logs
description: Watch, re-run, cancel and read the logs of workflow runs.
---

The runs view (`3` or `:runs`) lists workflow runs and refreshes every 5 seconds. Press `enter` on a run to open its [job graph](/gh-9s/guides/job-graph/).

## Run actions

Press these keys on a run. The same keys also work on the job graph and the job list.

| Key | Action |
|---|---|
| `enter` | Open the job graph of the run |
| `l` | Open the full log in the pager |
| `L` | Open the log of the failed jobs only |
| `w` | Watch the run (runs `gh run watch`) |
| `r` | Re-run (asks first) |
| `F` | Re-run only the failed jobs |
| `c` | Cancel |
| `d` | Delete (asks first) |
| `f` | Pin or unpin this repository |
| `o` | Open in the browser |

Each destructive action asks for confirmation and names the repository and run it applies to. If an action does not apply, gh-9s says why.

After you accept a re-run, gh-9s opens that run's graph and watches it again.

## Notifications

When a run you are watching finishes, gh-9s rings the terminal bell and shows a desktop notification.

## Reading logs

The log view supports these keys:

| Key | Action |
|---|---|
| `j` / `k` | Scroll one line |
| `space` / `ctrl+d` | Scroll a page |
| `/` | Search (smart-case) |
| `n` / `N` | Next / previous match |
| `f` | Follow a running job |

:::tip
On the job graph, `f` opens the log of the first failed job at its first error. This is the fastest way to find out why a run failed.
:::
