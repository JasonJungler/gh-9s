# Send an issue to a coding agent — design

## Intent

The user lives in a terminal multiplexer (Orca, herdr or tmux) and reads GitHub issues in gh-9s.
Today, handing an issue to a coding agent means copying the text by hand, opening a new workspace
or pane, starting the agent and pasting. This feature turns that into one key on an issue plus one
confirmation: gh-9s opens the right kind of space for the environment it is running in, starts the
agent, and gives it the issue as context.

## Goals

- From an issue (list row or detail screen), one key opens a one-line confirmation; `enter` launches.
- Works in three environments, picked automatically: **Orca**, **herdr**, **tmux**. The code is
  abstracted behind one interface so a fourth environment is one new implementation.
- Works with **Claude Code** and **Devin** first. Agents live in a table, so adding Codex, Gemini
  and so on is adding one row, not new control flow.
- The user can pick the **model** the agent starts with. The edit form lists the models known for
  the chosen agent and also accepts any typed model name, and the choice is passed to the agent
  at launch (Claude: its `--model`). Leaving it on "default" passes no model flag, so the agent
  uses its own default.
- The context handed over is identical for every agent and every environment:
  `請解決這個 issue` (a fixed sentence, in Chinese as the user wrote it), then the issue's title,
  body and **all** comments. "Solve an issue" and "implement a requirement" are not different
  modes.
- In Orca the launch creates a new workspace (worktree) on a chosen base branch, after a
  `git fetch`.
- In tmux and herdr the launch is two steps: open the pane, start the agent in it, wait until the
  agent is ready, then paste the context into it.

## Non-goals

- Not for pull requests, discussions or Project cards in this release (issues only; the interface
  should not make adding them hard). *(my call)*
- No branch or worktree handling in tmux and herdr: the new pane opens in the current directory on
  the current branch. *(my call, from the user's "open a pane, then pass the context")*
- No downloading of images in an issue; they stay as links. *(my call)*
- No agent supervision after launch (no waiting for the agent to finish, no reading its output back).

## Anti-goals (a failure even if it technically works)

- Silently launching on a stale base: if `git fetch` fails, stop and say so; never fall back to an
  old ref without telling the user.
- Pasting into the wrong place: the context must only be sent to the pane gh-9s just created, never
  the user's current pane. If readiness cannot be confirmed, do not paste; leave the pane open and
  say what happened.
- Hidden side effects: nothing is created before the user presses `enter` on the confirmation
  (the same "every change asks first" rule as the rest of gh-9s).

## What the user sees (decided)

Pressing the key on an issue opens a **one-line confirmation** (user chose this over a full form or a
two-step wizard):

```
┌ Send #42 ──────────────────────────────────────────────┐
│ orca ◆ new workspace · claude · origin/main → issue-42-fix-login-crash │
│ enter launch · e edit · esc cancel                     │
└────────────────────────────────────────────────────────┘
```

- `enter` launches with the shown values. `e` expands the line into a form where each value can be
  changed (typing for text fields, left/right to cycle choices). `◆` marks a value that was filled in
  automatically.
- In **Orca** the line shows target · agent · model · base → branch name, for example
  `orca ◆ new workspace · claude · default model · origin/main → issue-42-fix-login-crash`.
  Changing the agent in the edit form reloads the model list for that agent.
- In **tmux** the user decides, and the form offers: **pane or window**, and for a pane **horizontal
  or vertical split**. In **herdr** the equivalent choices are offered. The choices have defaults
  in `:settings` so the one-line form needs no edit when the defaults suit.
  (The user said "pane or panel"; "panel" is read as tmux's *window*. herdr's choice maps to its
  pane/tab. *my call*)

## Environment detection (my call)

By environment variables: Orca sets `ORCA_WORKTREE_ID` (and other `ORCA_*`), herdr sets
`HERDR_ENV` / `HERDR_PANE_ID`, tmux sets `TMUX` / `TMUX_PANE`. When several are present (for
example tmux started inside an Orca terminal), prefer Orca, then herdr, then tmux. A setting
(`launch_target`: auto | orca | herdr | tmux, default auto) overrides detection. When none is
present the key explains that no supported environment was found.

## Per-environment behaviour

**Orca** (my call: use Orca's own agent launch, not a second paste step, because Orca waits for the
agent itself):

1. `git fetch` for the remote of the chosen base (this goes in `internal/ghcli`, which is the only
   place that runs `git`).
2. `orca worktree create --name <branch name> --base-branch <base> --issue <N> --agent <id>
   [--model <id>] --prompt <context> --json`, run from the current directory so Orca infers the repo.
3. Read the result to show where it went (and, on a lost reply, `--retry-launch <operationId>`
   exactly as Orca's help prescribes).

Facts checked on this machine: `orca worktree create` has no separate branch flag; Orca derives the
branch from `--name` and adds its own prefix (this worktree: display name `agentic-workflow`, branch
`MurabitoB/agentic-workflow`). So the "Branch" field is really the workspace *name*, and the
confirmation should show the final branch name from the result rather than promise one.
Defaults (◆): base = the repo's base ref in Orca (`orca repo` base-ref setting), falling back to the
repo's default branch; name = `issue-<N>-<slug of title>`.

**herdr and tmux** (the user's two-step model):

1. Open the space: herdr via its workspace/pane commands, tmux via `split-window` or `new-window`,
   in the current directory, in the layout the user chose. Capture the new pane's id.
2. Start the agent in that pane (herdr: `herdr agent start --kind <kind> --pane <id>`; tmux: type
   the agent's start command into the pane with `send-keys`).
3. Wait until the agent is ready (herdr: `agent start` already waits; tmux: poll the pane's output
   for the agent's ready marker with a timeout — the marker per agent lives in the agent table).
4. Paste the context (herdr: `herdr agent prompt`; tmux: `load-buffer` + `paste-buffer`, which
   handles long comment threads and newlines; then Enter).

## Agents

A table, one row per agent: id, the name Orca/herdr know it by, the command tmux types to start it,
the ready marker tmux waits for, how that agent takes a model (Orca `--model <id>`; herdr
`agent start … -- --model <id>`; tmux the agent's own `--model <id>` on the start command), and its
list of known models.

**Model list (my call, because no tool offers one):** neither `claude`, `orca` nor `herdr` can list an
agent's models; `claude --model` takes an alias (`fable`, `opus`, `sonnet`) or a full name and Orca's
`--model` takes a provider model id. Web check (Anthropic's model-config page and the open request
"add `claude model list`", anthropics/claude-code #12612): there is still no such command, and the
in-session `/model` picker is itself a curated subset, so it is not an authoritative source either.
So the list is a curated one in the agent table. **For Claude it is the official aliases, not
version numbers:** `default`, `best`, `fable`, `opus`, `sonnet`, `haiku`, `opusplan`, `sonnet[1m]`,
`opus[1m]`. Aliases resolve to the latest model for the user's provider, so the list does not go
stale when a new model ships; the dialog may show what each resolves to as a hint, marked as
"on the Anthropic API" because it differs per provider. Full model names (and any other agent's
model ids) are typed in. Claude does not validate `--model` up front, so a mistyped name only fails in
the agent pane on the first request; the dialog cannot prevent that and should say so next to the
free-text field. Fetching a live list from a provider API is out of scope (it would need an API key
the user may not have).

**Each agent declares where its model list comes from: a fixed list, or a command.** Claude uses the
fixed list above. **Devin has a real command:** its docs (docs.devin.ai/cli/reference/commands) give
`devin models list --format json` ("for scripts"), which lists the models the user's account can
use, so Devin's list is live. If that command fails, is missing, or returns something unparseable,
fall back to Devin's documented aliases `opus`, `sonnet`, `swe`, `codex`, `gemini` (the docs say they
always resolve to the latest in that family) and keep free text. The list is fetched when the user
picks Devin in the form, not on every key press, and shows "loading…" while it runs. Devin takes a
model with `--model <name>` (or `DEVIN_MODEL`), takes an initial prompt as `devin [options] -- <prompt>`
or `--prompt-file`, and accepts fuzzy names (family, alias or partial name). **None of this was run
here: Devin is not installed on the author's machine, and the docs do not show the JSON shape of
`models list` or any ready marker for the interactive session.** Both are things the builder must
check on a machine that has Devin, and until then the Devin row stays marked unverified.
`--effort` (Orca can pass it) is not part of this release. The chosen model is remembered per agent. Claude and Devin are rows 1 and 2. `launch_agent` setting holds
the last choice (default claude). Devin is not installed on the author's machine, so its row cannot
be verified there; mark it unverified in code until it has run once.

## Context text

```
請解決這個 issue

# <title> (#<N>)
<url>

<body>

## Comments
### <author> (1/<count>)
<comment body>
…
```

Labels and assignees are left out. Comments are all included, in order, with their author. (my call
on layout; the user specified only "title, body, all comments, with that sentence first".)

## Structure (my call, follows the repo's existing seams)

- `internal/launch/`: the backend interface (open space → start agent → wait ready → send context),
  the Orca, herdr and tmux implementations, environment detection, the agent table, and the context
  builder. Every external program is run through an injectable seam (as `ghcli` already does with
  `lookPath` / `startCommand`) so tests never launch anything.
- `internal/ghcli/`: gets the `git fetch` call only.
- `internal/tui/`: a launch dialog (on the existing dialog/form code) and the key, registered on
  the issues list and issue detail screens; progress and failures shown in the dialog.
- `internal/settings/`: new settings `launch_target`, `launch_agent`, `launch_tmux_place`
  (pane | window), `launch_split` (horizontal | vertical).
- Docs: a guide page next to the existing ones, and the key in the keys reference.

## Failure behaviour

Each step reports its own failure by name (fetch, create workspace, open pane, start agent, agent
not ready, paste). A pane or workspace that was already created is left open for the user to take
over; nothing is cleaned up automatically. Nothing is retried silently.

## Decisions made after the builder check (all *my call* unless noted)

- **Large context in Orca.** The Orca path starts with `--prompt <context>`. The builder must spike it
  with a long multi-line, non-ASCII thread first. If it cannot carry it, fall back to
  `worktree create` (no prompt) → `orca agent launch` → `orca terminal send`, i.e. the same
  open-then-paste shape the other two environments use. The backend interface already allows both.
- **Readiness in tmux.** One fixed 30 s timeout (the same as herdr's own default), no setting. On
  timeout nothing is pasted; the pane stays open and the dialog says "agent not ready" so the user
  can paste by hand. There is no "paste anyway" override in this release. An agent's trust or login
  prompt counts as "not ready" on purpose.
- **Pane identity.** The new pane's id is captured when it is created (tmux `split-window -P -F
  '#{pane_id}'`, `new-window -P -F '#{pane_id}'`; herdr's equivalent) and every later command
  names that id explicitly. herdr and tmux are installed on the author's machine, so the exact
  command forms are verified against the real tools, not guessed; only Devin stays unverified.
- **Base branch.** The base is a remote-tracking ref `<remote>/<branch>`. The default comes from
  Orca's base ref for the repo, else `origin/<default branch>`. A bare branch name gets `origin/`
  if that exists. If the repo has no remote, skip the fetch and say "no remote, using the local
  branch" in the dialog (visible, never silent). The fetch runs in the directory gh-9s was started in.
- **Wrong repo.** gh-9s can switch repos with `:repo`. If the repo on screen is not the repo of the
  directory gh-9s runs in, the key refuses with a one-line explanation (the launch would otherwise
  start an agent in the wrong repo). Launching into another repo is out of scope for this release.
- **Context size.** Everything is sent. Above roughly 100 000 characters the confirmation line shows
  a size warning, and still sends. Comments by deleted users show as `ghost`.
- **Retry.** Never automatic. If Orca's reply is lost, the dialog says so and `enter` retries with
  the same `--retry-launch <operationId>`.

## Left to the builder

- Which key triggers it (must not collide with existing issue keys).
- Exact tmux ready markers and herdr/tmux command spellings; verify against the installed tools.
- How progress is drawn inside the dialog.
- Test strategy for the backends beyond the fake-command seam.
