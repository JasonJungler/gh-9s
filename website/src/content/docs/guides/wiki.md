---
title: Wiki
description: Read a repository's wiki pages, follow links between them, edit them in your editor, add pages and delete pages.
---

The Wiki view (`6` or `:wiki`) lists the pages of the current repository's wiki: `Home` first, then the others by title, with who changed each one last and when. Press `enter` to read a page.

GitHub has no API for wikis. A wiki is a git repository (`OWNER/REPO.wiki.git`) and a page is a file in it, so gh-9s keeps a copy of that repository in your cache directory (under `gh-9s/wiki`) and brings it up to date every time you open the view or press `R`. The copy is only a cache; gh-9s throws it away and clones it again if it is ever damaged. Git is signed in through `gh auth git-credential`, so the token of `gh` is used and only for the host of the repository; a self-hosted GitHub works the same way as everywhere else in gh-9s.

## List

| Key | Action |
|---|---|
| `enter` | Read the page |
| `e` | Edit the page |
| `n` | Add a new page |
| `x` | Delete the page (asks first) |
| `/` | Filter by title (file names and authors are not searched) |
| `o` | Open the page, or the wiki, in the browser |
| `R` | Update from GitHub |

At most 500 pages are listed; when there are more, the status line says so. Pages that are links to somewhere else (symbolic links) are marked `[link]` and are never followed.

## Reading a page

Markdown pages are drawn like issue text. Pages in the other formats GitHub knows (`.txt`, `.textile`, `.asciidoc`, `.rst`, `.org` and so on) are shown as plain text. Links to other pages — `[[Page]]`, `[[Text|Page]]` and relative Markdown links — are listed under the text.

| Key | Action |
|---|---|
| `j` / `k` | Scroll |
| `tab` / `shift+tab` | Select the next / previous link |
| `enter` | Follow the selected link; a link to a page that does not exist says so |
| `esc` | Back along the links you followed, then to the list (standing on the page) |
| `e` | Edit the page |
| `x` | Delete the page (asks first) |
| `o` | Open in the browser |

## Editing

`e` reads the page afresh from GitHub, opens it in `$VISUAL` (else `$EDITOR`, else `vi`) and, when you close the editor, asks one question: the commit message, filled in with `Update <Page>` for you to keep or change. The question's title names the wiki and the page and says how many lines were added and removed. Sending it saves the page: a commit pushed straight to the wiki (a wiki has no pull requests).

- If you changed nothing, nothing is saved.
- A page cannot be emptied this way; use `x` to delete it.
- If someone else changed the same page on GitHub while you were editing, nothing is saved. Your text is written to a file under the cache directory (`gh-9s/wiki/lost/...`), the box that tells you says where, and the page is read again.
- If someone changed a different page, your change goes on top of theirs.
- If GitHub refuses the push (for example you cannot write to the wiki), the same happens: the error is shown in full together with the file that holds your text.
- If you press `esc` on the commit-message question, nothing is saved but your text is kept in a file there too, and the status line says where.
- Whatever the reason a save fails (GitHub unreachable, the page changed, a refused push), your text is kept in a file first.
- A page that uses CRLF line endings keeps them, whatever your editor writes.
- Your text is never taken from the cache copy: the editor works on a temporary file, so `R` or leaving the screen cannot lose it.

Only one wiki change can be under way at a time; `e`, `n`, `x` and `R` say so while one is.

## New page

`n` asks for the title, then opens an empty editor, then asks for the commit message (`Create <Title>`). The title becomes the file name: spaces turn into `-` and `.md` is added. A title is refused when it is empty, longer than 100 characters, starts with `.`, `-` or `_`, contains `-` (write a space, because GitHub shows dashes in file names as spaces), contains any of `/ \ : * ? " < > |` or a control character, or matches an existing page (whatever the case or format). A page with nothing written in it is not created.

GitHub creates a wiki together with its first page. If a repository has its wiki turned on but no page yet, the view says so and the first page has to be made in the browser (`o`); `n` explains that too. If the wiki is turned off for the repository, the view says that.

## Delete

`x` asks first, naming the page and the repository: "Delete the page "Old notes" from the acme/api wiki?". Deleting `Home` carries an extra warning that the wiki then has no front page. The page stays in the wiki's git history. If the page changed on GitHub after you looked at it, it is not deleted.

Everything GitHub sends is cleaned of control characters before it is drawn. Pages larger than 1 MB are neither shown nor edited, and pages that are not text are not shown.

Not in this release: renaming or moving pages, folders, uploading images or attachments, a history or diff view, searching inside pages, and creating the very first page of a wiki.
