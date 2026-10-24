# gh9s

A [k9s](https://k9scli.io)-style terminal UI for GitHub Actions and pull requests, as a `gh` extension.

## Install / develop

```sh
mise install                 # pins go + gh (see mise.toml)
mise exec -- go build -o gh9s .
gh extension install .       # run as `gh 9s`... see note below
```

`gh` extensions must be named `gh-<name>`; for `gh gh9s`, build the binary as `gh-gh9s`
or publish the repo as `gh-9s`. Run inside any git repo that has a GitHub remote.

## Views

| key | view |
|-----|------|
| `1` / `:runs` | workflow runs (auto-refresh every 5s) |
| `2` / `:wf` | workflows |
| `3` / `:pr` | open pull requests (`*` marks the current branch) |
| `P` / `:cpr` | checks of the current branch's PR |

## Keys

`j/k` move · `g/G` top/bottom · `/` filter · `:` command · `esc` back · `R` refresh · `o` open in browser · `q` quit

- Runs: `enter`/`l` logs, `L` failed logs, `w` watch (`gh run watch`), `r` rerun, `F` rerun failed, `c` cancel, `d` delete
- Workflows: `enter` runs of that workflow, `t` trigger (`<ref> [key=value ...]`), `e` enable/disable
- Pulls: `enter` checks
