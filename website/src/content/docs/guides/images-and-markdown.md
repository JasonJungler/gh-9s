---
title: Images and Markdown
description: How descriptions, comments and code blocks are rendered, and how images are shown.
---

## Markdown

PR and issue descriptions and comments are rendered as formatted text. The following are supported:

- Headings, lists and task lists
- Tables
- Code blocks
- Quotes

Code blocks are coloured by the language written after the fence (```` ```go ````). Blocks without a language, or with an unknown one, stay plain.

## Images

Images appear as `[image: alt text]` markers in the text.

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Select an image |
| `enter` | Show the image full screen |
| `esc` or `q` | Leave the full-screen image |
| `o` | Open the image in the browser (on the full-screen image), or the PR or issue otherwise |

gh-9s fetches images with your `gh` login, only from your GitHub host. It accepts PNG, JPEG and GIF files up to 5 MB.

### How an image is drawn

The picture is drawn with the best option your terminal supports:

- **iTerm2:** inline images
- **Windows Terminal:** Sixel graphics
- **Everywhere else** (macOS Terminal, VS Code, SSH): coloured half-block characters

Inside tmux, graphics are used only when tmux's `allow-passthrough` option is on. Otherwise you get the half-block picture.

Set `GH9S_IMAGES` to override the choice:

| Value | Effect |
|---|---|
| `auto` | Choose by terminal (default) |
| `iterm` | Force iTerm2 inline images |
| `sixel` | Force Sixel |
| `blocks` | Force half-block characters |
| `off` | Send images to the browser instead |

## Diffs

Code in diffs is coloured by language in both the side-by-side and the single-column view. See [Reviewing a PR](/gh-9s/guides/reviewing-prs/).
