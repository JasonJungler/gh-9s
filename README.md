# gh9s

A [k9s](https://k9scli.io)-style terminal UI for GitHub Actions and pull requests, as a `gh` extension.

## Install / develop

```sh
mise install                          # pins go + gh (see mise.toml)
mise exec -- go build -o gh-9s .      # binary must be named after the repo: gh-9s
gh extension install .                # then run: gh 9s
```

Publish the repo as `gh-9s` (tag `v*` triggers the precompiled release workflow).
Run inside any git repo with a GitHub remote.

## Views

| key | view |
|-----|------|
| `1` / `:runs` | workflow runs (auto-refresh every 5s) |
| `2` / `:wf` | workflows |
| `3` / `:pr` | open pull requests (`*` marks the current branch) |
| `enter` on a run | jobs of that run, then `enter` again for the job's log |
| `P` / `:cpr` | checks of the current branch's PR |

## Keys

`j/k` move · `g/G` top/bottom · `/` filter · `:` command · `esc` back · `R` refresh · `o` open in browser · `q` quit

- Runs: `enter` jobs → logs, `l` full log in pager, `L` failed logs in pager, `w` watch (`gh run watch`), `r` rerun, `F` rerun failed, `c` cancel, `d` delete
- Workflows: `enter` runs of that workflow, `t` trigger (`<ref> [key=value ...]`), `e` enable/disable
- Logs: `j/k` scroll, `space`/`ctrl-d` page, `g/G` top/bottom, `f` follow a running job
- Pulls: `enter` checks
