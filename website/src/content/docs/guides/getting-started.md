---
title: Getting started
description: Install, upgrade and run gh-9s.
---

gh-9s is a [k9s](https://k9scli.io)-style terminal UI for GitHub, distributed as a `gh` extension.

## Prerequisites

- **[GitHub CLI (`gh`)](https://cli.github.com/)**, installed and logged in. Run `gh auth login` if `gh auth status` fails.
- **git**, and a repository with a GitHub remote to run gh-9s in.
- **A wide terminal is recommended.** Side-by-side diffs need at least 120 columns, and the preview pane needs 120 or more as well. Narrower terminals fall back to single-column views.

For building from source you also need:

- **[mise](https://mise.jdx.dev/)**, which installs the pinned Go and gh versions from `mise.toml`.

## Install

### From GitHub releases (recommended)

Prebuilt binaries are published with each release:

```sh
gh extension install MurabitoB/gh-9s
```

Then run it inside any git repository with a GitHub remote:

```sh
gh 9s
```

### From source

```sh
mise install                          # pins go + gh (see mise.toml)
mise exec -- go build -o gh-9s .      # the binary must be named after the repo: gh-9s
gh extension install .                # then run: gh 9s
```

## Upgrade

Upgrade an installed release to the latest version:

```sh
gh extension upgrade gh-9s
```

To upgrade every extension you have installed:

```sh
gh extension upgrade --all
```

:::note
An extension installed from source (`gh extension install .`) is not upgraded this way. Rebuild it from the repository instead, or reinstall from GitHub releases.
:::

## Authentication

All GitHub access goes through the `gh` CLI, so authentication, proxies and GitHub Enterprise follow whatever `gh` is logged in to (or `GH_HOST`). Make sure `gh auth status` succeeds first.

:::note
GitHub Enterprise has not been tested yet.
:::
