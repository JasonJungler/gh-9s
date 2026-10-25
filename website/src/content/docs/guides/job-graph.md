---
title: Job graph
description: How gh-9s draws the jobs of a workflow run and its dependencies.
---

Press `enter` on a run to open its job graph. It shows each job, its status, and the `needs:` dependencies between jobs.

## Moving around

- `h` `j` `k` `l` or the arrow keys move between jobs.
- `enter` opens the log of the selected job.
- `v` switches the graph style.
- `ctrl+d` and `ctrl+u` scroll.
- `f` opens the log of the first failed job at its first error.
- `r`, `F`, `c` and `d` re-run, re-run failed, cancel and delete the run, as on the [runs list](/gh-9s/guides/runs-and-logs/).
- `o` opens the run in the browser.

## Styles

Press `v` to cycle through three styles:

- **Boxes** (default): left to right, with arrows between stages.
- **Tree**: top to bottom. Fits narrow terminals.
- **Lanes**: a timeline.

If the boxes are too wide for the terminal, gh-9s falls back to the tree.

## Dependencies

- In the boxes style, lines connect only neighbouring stages. A dependency that skips a stage is written inside the box instead, for example `build 4✓ ← lint`. A line is never a guess.
- The tree style spells out every `(needs …)`.
- Dependencies come from the `needs:` entries of the workflow file at the run's commit. If they cannot be determined, gh-9s shows a plain job list and says why. It never draws a guessed graph.

## Matrix and reusable workflows

A matrix job or a reusable-workflow job is collapsed into one node that shows counts by status, for example `6✓ 1✗ 2●`.

- `enter` expands the node.
- `x` collapses it again.
