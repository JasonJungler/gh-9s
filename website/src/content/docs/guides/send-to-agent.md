---
title: Send an issue to an agent
description: Press a on an issue to open a workspace or pane in Orca, herdr or tmux, start Claude Code or Devin there, and give it the issue.
---

Press `a` on an issue to hand it to a coding agent. gh-9s opens a new workspace (Orca) or pane (herdr, tmux), starts the agent in it and gives it the issue as its first message (or, with [paste only](#paste-only-instead-of-submitting), puts it in the agent's input box for you to send). It works from the issue list and from the issue screen. Only issues are supported, not pull requests.

Nothing is created until you press `enter` on the confirmation.

By default the issue is sent to the agent at once. If you would rather read it first and press `Enter` yourself, turn off the `launch_submit` setting: see [Paste only instead of submitting](#paste-only-instead-of-submitting).

Devin support is unverified. Devin was not available when this was written, so its ready check and its model list have never run against the real program. Claude Code has been run. See [Devin](#devin).

## The confirmation

`a` opens a dialog with one line that says what will happen. For example, in Orca:

```
orca ◆ new workspace · claude · default model · origin/main → issue-42-fix-login-crash
```

and in tmux:

```
tmux ◆ new pane (right) · claude · default model
```

`◆` means the environment was detected automatically. It becomes `·` once you pick one yourself. When `launch_submit` is off, the line ends with `· paste only`. For Orca, when `launch_orca_nested` is on, it also says `· child of current` (before `paste only`); nothing is added when the workspace is independent.

| Key | Action |
|---|---|
| `enter` | Launch with what the line shows |
| `e` | Edit the values |
| `esc` | Cancel. Nothing is created. |

On the issue list, gh-9s first loads the whole issue (a row only has its number), so `a` can take a moment.

### Editing

`e` turns the line into a form. Use `tab` or `↑` `↓` to move, `←` `→` to choose a value, and type to edit a text field. `enter` launches from the form too, except on the Prompt row, where it opens the editor (see [Editing the prompt](#editing-the-prompt)). The rows are:

| Row | Applies to | What it sets |
|---|---|---|
| Where | all | `orca`, `herdr` or `tmux` |
| Agent | all | `claude` or `devin`. Changing it reloads the model list. |
| Model | all | A model from the list, or any name you type. `default` passes no model flag, so the agent uses its own default. |
| Base | Orca | The ref the new workspace branches from |
| Branch | Orca | The workspace name, `issue-<number>-<title words>` to start with |
| Place | herdr, tmux | `pane` or `window` (a tab, in herdr) |
| Split | herdr, tmux, for a pane | `horizontal` (new pane on the right) or `vertical` (below) |
| Prompt | all | A summary of the text the agent will get, such as `title + body + 2 comments · 1840 characters`. `enter` edits it in `$EDITOR`. |

Base and Branch are required in Orca. If one is empty, `enter` says so instead of launching.

### Editing the prompt

The Prompt row is the last row of the form. It shows what will be sent, as a summary, for example `title + body + 2 comments · 1840 characters`. To change the text itself, focus the row and press `enter`. gh-9s opens the **whole prompt** in your editor: the opening sentence, the title, the description and all comments, exactly as [the agent would receive it](#what-the-agent-receives). It uses `$VISUAL`, then `$EDITOR`, then `vi`, the same as writing a comment draft with `ctrl+e`. Save and quit to use the text.

| Key on the Prompt row | Action |
|---|---|
| `enter` | Open the prompt in `$VISUAL` / `$EDITOR`. The editor shows the current prompt, so a second edit starts from the first. |
| `r` | Go back to the generated prompt. |

Once the prompt has been changed, the row reads `edited · N characters` and the one-line summary gets `· prompt edited`. The edited text is used for everything that depends on it: what the agent receives, the `⚠ large issue` character warning, the byte limit warnings for Orca and herdr, and the check that refuses a text too big for the platform ([What Orca does differently](#what-orca-does-differently)).

What happens to the text that comes back from the editor:

- Windows line breaks (`CRLF`) become ordinary line breaks first, so an editor that saves them does not lose your lines.
- Control characters other than newline and tab are removed, just as they are from the issue text: carriage returns, escape sequences and the end marker of a bracketed paste cannot reach the agent. Invalid UTF-8 becomes the replacement character `�`.
- Trailing spaces and blank lines at the end are removed.
- A text that is empty after that is refused with `prompt is empty`, and the previous prompt is kept. Saving without a change, an editor that fails or exits with an error, and a text equal to the generated one also leave the prompt as it was (the last one clears the `edited` mark).

The edit belongs to this one launch. It is not saved in the settings, in the remembered model file or in your drafts; the next time you press `a` the prompt is generated again. Changing Where, Agent, Model, Base, Branch, Place or Split does not discard it. While the editor is open nothing can be launched or cancelled; `esc` works as before once it has closed. A prompt that begins with `-` (a Markdown bullet, `--help`) is refused for Orca, and for herdr when the prompt is submitted, because they could read it as an option: the dialog says so and nothing is created. Start the prompt with a word. Pasting without submitting in herdr is not affected, and neither is tmux.

The opening sentence, `Please solve this issue`, is fixed. There is no setting for it: `:settings` can only hold on/off, choice and number values, not free text. Edit the prompt to change it for one launch.

After a successful launch, gh-9s remembers the agent (in the `launch_agent` setting) and the model you used for it. The next dialog starts from them.

## Which environment

gh-9s looks at the environment variables of the terminal it runs in. When more than one is present, the first match wins:

1. **Orca**, if `ORCA_WORKTREE_ID` is set
2. **herdr**, if `HERDR_ENV` or `HERDR_PANE_ID` is set
3. **tmux**, if `TMUX` is set

So tmux started inside an Orca terminal counts as Orca. To override this, set `launch_target` to `orca`, `herdr` or `tmux` (or choose Where in the edit form for one launch). If none is found and the setting is `auto`, `a` shows `no supported environment found (orca, herdr or tmux)`.

The new pane or window opens in the directory gh-9s was started in, on whatever branch is checked out there.

### Only the current directory's repo

If you switched repos with `:repo`, so that the repo on screen is not the repo of the directory gh-9s runs in, `a` refuses and says so. Otherwise the agent would start in the wrong repo.

## What Orca does differently

Orca gets a new workspace (a worktree) rather than a pane in your current directory.

1. gh-9s runs `git fetch` for the remote of the base, so the workspace starts from what the remote has now. A bare base such as `main` means the same branch on `origin`: gh-9s fetches `origin` and gives Orca `origin/main`, so the workspace starts from what the remote has and not from a local `main` that may be behind. If `origin` has no branch of that name but the name is a local branch, a tag or a commit id, that ref is used as it is and the launch shows the note `origin has no <base>; using the local ref <base>`. If the name is none of these, the launch stops at `fetch` and says the base was not found. A base that already names a remote, such as `upstream/main`, or that starts with `refs/`, is used as it is. If the repo has no remote, there is nothing to fetch and the launch continues from the local branch under its bare name, with the note `no remote, using the local branch`. If the fetch fails, the launch stops. It never falls back to an old ref without telling you.
2. gh-9s asks Orca to create the workspace, link it to the issue and start the agent with the issue as its prompt. Orca does the starting and waiting itself. (With `launch_submit` off, Orca takes the longer path instead, see below.)

New workspaces are independent by default. Orca normally infers a parent from where a command runs, so a workspace created from inside another Orca workspace would become its child (a "sub workspace"). gh-9s passes `--no-parent` to `orca worktree create` to prevent that, so the workspace is a top-level one. Turn on `launch_orca_nested` (`:set launch_orca_nested on`) and gh-9s passes nothing, so Orca uses its inferred parent: the workspace gh-9s runs in. The setting is read when the dialog opens, and a retry after a lost reply (`--retry-launch`) uses the same value as the launch it repeats. `--no-parent` only changes the lineage Orca shows. It does not change the git branch or the base the workspace starts from. It is not shown as a row in the edit form.

**Verification:** the `--no-parent` flag is in `orca worktree create --help` (read only), and the exact arguments for both settings values, on the one-call path, the longer path and the retry, are checked by unit tests against a fake `orca`. The effect in a real Orca (that a workspace made from inside another one is a top-level workspace with the setting off and a child with it on) has not been verified live, because that would create workspaces.

The base defaults to the base ref Orca has for the repo, or else `origin/<the default branch>` (`origin/main` if git cannot tell). In a repo with no remote there is no `origin/...`, so it is the branch that is checked out, and the dialog shows `no remote, using the local branch`. The base is looked up when the dialog opens. If you press `enter` before it arrives, nothing is launched: the dialog says `still finding the base branch…` and you press `enter` again once the base shows.

Orca decides the branch name: it adds its own prefix to the workspace name you give. So the Branch row is really the workspace name. After the launch, the message shows the real branch Orca made.

Orca and herdr receive the issue as a program argument, and the operating system limits that in bytes. gh-9s refuses a larger text before it runs anything: the launch fails at `create workspace` and says how many bytes there are and what the limit is. Trim the issue, or use tmux, which gets the text on standard input and has no such limit.

| Platform | Most bytes of issue text |
|---|---|
| Linux | 131,071 |
| macOS | 524,288 |
| Other | 30,000 |

The limit counts bytes, not characters: Chinese, Japanese and emoji take 3 or 4 bytes each, so 40,000 Chinese characters are already about 120,000 bytes. When the issue is near the limit (90% or more) or over it, the confirmation line says so. With `launch_submit` off, herdr also sends 12 bytes of paste markers around the text in the same argument (see below), and those count too; the line shows the size with them. Only Orca and herdr show the byte count and the limit there; tmux has no such limit and shows nothing about bytes.

On macOS, an Orca issue of more than 131,071 bytes cannot go in one `create` call. gh-9s then creates the workspace, starts the agent, waits for it to be idle and sends the text to its terminal, which gives the same result. On Linux the two limits are the same, so that path is never needed.

## tmux and herdr

These work in two steps, in a pane that gh-9s creates and names explicitly. The text is never sent to your current pane.

1. Open the pane (or window, or herdr tab) in the layout you chose.
2. Start the agent in it.
3. Wait until the agent is ready, for up to 30 seconds.
4. Paste the issue and press `enter` (or, with `launch_submit` off, only paste it).

If the agent is not ready in time, nothing is pasted and the pane stays open, so you can paste by hand. An agent that stops at a trust or login prompt counts as not ready on purpose.

In tmux there is one more check before the paste, in both modes: the agent must have turned on bracketed paste (see [Paste only instead of submitting](#paste-only-instead-of-submitting) for how the tmux check works). If it has not within 5 seconds, nothing is pasted and `Enter` is not pressed.

How readiness is detected differs. tmux watches the pane for the agent's ready text (for Claude Code, its `Claude Code v` banner) and checks that the pane is no longer running a shell. herdr's own `agent start` waits; a timeout there is reported as `start agent`.

## Paste only instead of submitting

The `launch_submit` setting (default on) decides what happens to the issue once the agent is ready. With `off`, gh-9s only places the text in the agent's input box. You read it, edit it if you like, and press `Enter` in the agent yourself. It is one global setting, not one per environment. Change it in `:settings` or with `:set launch_submit off`.

When it is off, the confirmation line ends with `· paste only`, and after a successful launch the message ends with `paste only: press Enter in the agent`. gh-9s itself never presses `Enter`. Whether a newline in the text could still submit it depends on the agent having turned on bracketed paste (see below); tmux checks that, in both modes, and refuses if it is not on.

How each environment does it:

| Environment | What gh-9s does | Status |
|---|---|---|
| tmux | First waits, for up to 5 seconds, for the pane to report that its program has turned on bracketed paste (`#{bracket_paste_flag}` is `1`). Then it pastes the text with `paste-buffer -p` and skips the final `Enter`. If the program has turned bracketed paste on, a multi-line text arrives as one paste, so it stays in the input box. If the flag never becomes `1`, gh-9s pastes nothing and fails at `paste` with `the agent has not turned on bracketed paste; nothing was pasted or sent, so the text did not reach it`. Without bracketed paste, tmux would send each line feed as Enter and the first line would submit. The wait is there because an agent's ready banner can show before it has turned bracketed paste on. **The same wait and refusal apply with `launch_submit` on**: without bracketed paste the first line of a multi-line text would be submitted on its own and the remaining lines would arrive as separate messages, so gh-9s pastes nothing and presses no `Enter`. | Checked against a real tmux server with a program that echoed its raw input: the bracketed paste arrived and nothing else, and the flag read `0` for a plain program and `1` after it turned bracketed paste on. Checked with a real Claude Code under tmux: the flag turned `1` as its banner appeared (about a second after start) and a pasted multi-line text stayed in its input box, unsubmitted. **Unverified:** Devin. If a program never turns bracketed paste on, the launch fails at `paste` and nothing is placed or sent, with `launch_submit` on or off. |
| herdr | `herdr agent prompt` always presses `Enter` and has no option to skip it, so gh-9s uses `herdr pane send-text` instead. | Checked live (herdr 0.8.2), see below. |
| Orca | Always uses the longer path: create the workspace without an agent, launch the agent, wait until it is idle, then `orca terminal send --text` **without** `--enter`. This is the path already used for an issue too big for one `create` call. Compared with the normal Orca path it adds the `start agent` and `agent not ready` steps (see [The longer Orca path](#the-longer-orca-path)) and has no retry after a lost reply. | Checked against a real Orca (1.4.238) with Claude Code: a multi-line Chinese and English text stayed in Claude's input box as one input and nothing was submitted. Orca writes the bytes as they are, without bracketed paste markers; a plain line feed does not submit in Claude Code, and gh-9s removes carriage returns and other control characters from the issue text. **Unverified:** Devin, or any agent that treats a line feed as Enter, could submit at the first line break. |

### herdr

`herdr pane send-text` was tried in a throwaway pane running a program that echoed the raw bytes it received. It writes the text exactly as given, adds no newline and no `Enter`, and does not make it a bracketed paste, even when the program asked for bracketed paste. A newline in the text would therefore arrive as a plain line feed, which an agent can take for `Enter` and submit.

So gh-9s wraps the text itself in the bracketed paste markers (`ESC[200~` before and `ESC[201~` after), so the agent reads the whole issue, newlines included, as one paste. Control characters are already removed from the issue text, so the text cannot contain the closing marker; as a second check, herdr refuses to send a text that has any control character other than newline and tab, and sends nothing. This relies on the agent having turned on bracketed paste, as agent terminal interfaces normally do; Claude Code and Devin are expected to, but that is not verified. Only the echo program was used to check the bytes: no real Claude Code or Devin was run, so whether they keep the pasted text in their input box is not verified. Unlike tmux, gh-9s cannot ask herdr whether the program turned bracketed paste on, so it does not check. A program that has not turned on bracketed paste may show the markers as characters or submit at a newline.

### The longer Orca path

Because Orca starts the agent and sends the text in separate calls, the launch takes a little longer and has the extra steps `start agent` (the agent launch) and `agent not ready` (the wait). Unlike the normal Orca path, a lost reply from `agent launch` is a plain failure with no retry: the workspace already exists, so the dialog tells you to start the agent from Orca by hand. The text is still a program argument, so the byte limit above applies as before.

## What the agent receives

The same text for every agent and every environment:

```
Please solve this issue

# <title> (#<number>)
<url>

<description>

## Comments
### <author> (1/<count>)
<comment>
…
```

All comments are included, in order. With `launch_submit` off the agent receives the same text; it is only not submitted. A comment from a deleted user shows `ghost`. An issue with no description shows `(no description)`. Labels and assignees are left out, and images stay as links. Control characters are removed so issue text cannot send terminal commands to the agent's terminal.

You can change this text before the launch, see [Editing the prompt](#editing-the-prompt).

Above 100,000 characters the line adds `⚠ large issue, N characters`. In Orca and herdr the byte limit above applies as well. Nothing is cut: an issue is either sent whole or refused.

## Models

Neither Claude Code nor Orca nor herdr can list an agent's models, so for Claude the list is fixed. It holds the official aliases, which always point at the latest model of their kind, so the list does not go stale:

`default`, `best`, `fable`, `opus`, `sonnet`, `haiku`, `opusplan`, `sonnet[1m]`, `opus[1m]`

You can type any other model name in the Model row, such as a full model name. gh-9s does not check that the model exists. **A mistyped name only fails inside the agent's pane**, on its first request. In tmux, where the name is typed into your shell, the name must start with a letter or digit and may then have only letters, digits and `. _ : / @ + [ ] -`. Anything else fails at `start agent` before a pane or window is opened, so nothing is left behind.

### Devin

Devin's list comes from `devin models list --format json`, which lists the models your account can use. It is loaded when you pick Devin, and the list shows only `default` until it arrives. If the command fails, is missing, takes more than 5 seconds or prints something gh-9s cannot read, the list falls back to `opus`, `sonnet`, `swe`, `codex` and `gemini`. Typed names work as well.

Unverified: Devin was not installed where this was written. The shape of the JSON that `devin models list` prints, and the text gh-9s waits for in tmux before pasting (`❯`), are guesses from its documentation. If Devin does not work for you, that is the first place to look.

## When a step fails

The dialog stays open and names the step that failed:

| Step | Meaning |
|---|---|
| `fetch` | `git fetch` failed (Orca). No workspace was created. |
| `create workspace` | Orca could not create the workspace, or the request was refused before anything ran (for example, an empty or unsafe name, or text too large to pass) |
| `open pane` | The pane, window or tab could not be opened |
| `start agent` | The agent could not be started, or (herdr) did not become ready, or (Orca's longer path) the agent launch was not confirmed |
| `agent not ready` | The agent did not become ready in 30 seconds (tmux, and Orca's longer path, where Orca's wait for the agent to be idle timed out). Nothing was pasted. |
| `paste` | The agent is running but the text could not be sent or placed in its input, for example in tmux because the agent has not turned on bracketed paste within 5 seconds (with `launch_submit` on or off; nothing is pasted and `Enter` is not pressed) |

Whatever was already created is left open for you to take over. Nothing is cleaned up and nothing is retried silently. `enter` in the failed dialog tries again from the start, which can make a second pane or workspace, so check what is already open first.

The one exception is an Orca launch whose reply was lost. The agent may already be running, so the dialog says so and `enter` asks Orca about that same launch again instead of starting a new one. The retry does not run `git fetch` again. If the retry itself fails, the dialog keeps offering the same retry, so you never start a duplicate by pressing `enter` again. Editing a field or pressing `esc` discards the retry.

## Settings

Five settings choose the defaults. See [Settings](/gh-9s/guides/settings/).

| Setting | Default | Values |
|---|---|---|
| `launch_target` | `auto` | `auto`, `orca`, `herdr`, `tmux` |
| `launch_agent` | `claude` | `claude`, `devin` |
| `launch_tmux_place` | `pane` | `pane`, `window` |
| `launch_split` | `horizontal` | `horizontal`, `vertical` |
| `launch_submit` | `true` | on / off |
