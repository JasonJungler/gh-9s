# gh-9s: GitHub Wiki

Status: draft, waiting for approval. Nothing is built.

## Intent

The user wants to read a repo's Wiki pages and edit them without leaving the terminal, as they already do for issues,
pull requests and Discussions. In their words: view and edit pages; creating and deleting pages too. One release,
reviewed by one fresh Opus reviewer, published only on the user's explicit "發佈".

## What GitHub gives us (checked, not assumed)

- GitHub has no Wiki API. A wiki is a git repo, `https://HOST/OWNER/REPO.wiki.git`; a page is a file (`Home.md`,
  `Color-schemes.md`); the page title is the file name with `-` turned into a space. Other formats exist (`.textile`,
  `.asciidoc`, `.rst`, `.org`, ...). Special pages: `Home`, `_Sidebar`, `_Footer`.
- A shallow clone of `junegunn/fzf.wiki.git` takes under a second.
- A repo with the wiki turned on but no page yet has **no** wiki repo: `git clone` answers "Repository not found" (seen
  with the user's own `MurabitoB/gh-9s`). The first page can only be made in the browser.
- There is no pull request for a wiki: a save is a commit pushed straight to the wiki's default branch.
- `hasWikiEnabled` is readable with `gh repo view --json hasWikiEnabled` (`has_wiki` in the REST repo object).

## Goals

1. **List** (`6`, `:wiki`): the pages of the current repo's wiki: `PAGE` (title), `FILE`, `UPDATED` (last commit
   touching it), `AUTHOR` (of that commit). `Home` first, then alphabetical; `/` filters by title. Up to 500 pages, noted when
   cut. `o` opens the wiki in the browser, `R` updates from GitHub.
2. **Read** (`enter`): the page as formatted Markdown (the same renderer as issues), with `[[Page]]`,
   `[[Text|Page]]` and relative links to other pages shown as links that `tab`/`shift+tab` select and `enter` follows (to
   that page, or "no such page" on the status line). Pages in other formats are shown as plain text, with a note. `esc` goes back
   to the same place in the list.
3. **Edit** (`e` on a page): opens the page in `$VISUAL`/`$EDITOR` (the mechanism the comment dialogs' `ctrl+e` uses). When the
   editor closes: unchanged text means "no changes"; otherwise gh-9s shows how many lines were added and removed and asks
   for a commit message in one dialog (empty = `Update <Title>`), whose title names the wiki and the page
   (`Save Home to acme/api wiki`); sending that dialog is the confirmation. Then it commits and pushes.
4. **New page** (`n` in the list): asks for the title, turns it into a file name (spaces to `-`, `.md`), refuses names
   that already exist (case-insensitively), are empty, start with `.` or `-`, are longer than 100 characters, or contain
   `/ \ : * ? " < > |` or control characters; then the same editor and the same message dialog (empty = `Create <Title>`).
   An empty page is not created ("nothing written").
5. **Delete page** (`x` in the list or on the page): a confirm that names the wiki and the page and says that it is removed for everyone
   and stays only in the wiki's git history (and, for `Home`, that the wiki then has no front page); `y` confirms; then it
   commits `Delete <Title>` and pushes.
6. **Not there / not allowed**, said in words, never as a raw git error: wiki turned off for the repo; wiki on but no pages yet
   ("GitHub creates the wiki with its first page: make it in the browser (o)"; `n` is refused for the same reason); no write
   access (push refused); host unreachable.
7. Enterprise hosts work like everywhere else: the repo's host is used for the URL and for the credentials.

## Non-goals / anti-goals

Non-goals: renaming or moving pages, folders (a page name has no `/`), uploading images or attachments, history or diff
views, searching inside pages, previewing unsaved text, editing in a built-in text area, off-line editing queues.
Anti-goals: overwriting someone else's newer edit of the same page; losing the user's text when a save fails; the token
going anywhere but the repo's own host over https; a page file name that escapes the clone directory or follows a symlink out
of it; pushing or deleting without the question; two writes at once.

## Constraints

Go 1.27 via mise (`mise exec -- go ...`), existing libraries only. Follow the codebase's rules: screens register in the registry;
outside calls are package vars (seams) tests replace; identity (repo, page, the version the user opened) is captured at
key-press time and a stale async result is dropped by identity; remote text goes through `ghcli.CleanText`; every
write asks first naming the exact change; one write in flight; every call has a timeout. Only `internal/ghcli` runs
`gh` or `git`; the commands that change the wiki live in one write file, and the existing test that restricts where writes
appear is extended to it. Keys appear in `?` help, the footers, the README and the website's guide and key reference.

## How it works (my calls, please check)

- **Local copy:** a full clone (history is small) under `os.UserCacheDir()/gh-9s/wiki/HOST/OWNER/REPO.wiki`; it is a cache: when
  it is damaged, dirty or has diverged, it is deleted and cloned again. Entering the screen and `R` run `git fetch` and move the
  working tree to the remote's default branch (`git reset --hard origin/<branch>`).
- **Git calls:** `git` with `GIT_TERMINAL_PROMPT=0`, `-c credential.helper=` to drop other helpers, then
  `-c credential.helper=!gh auth git-credential` so the token is `gh`'s own and is only given to the host in the URL;
  the URL is built from validated owner/name and the host, https only. Commit identity is the user's `git config user.name/email`;
  if either is missing, `LOGIN <LOGIN@users.noreply.github.com>` with the login from `gh`.
- **Conflict rule:** when the editor closes, fetch again. If the page's content on the remote differs from what the user
  opened (compared by blob id), nothing is committed: the user's text is saved to a recovery file
  (`.../gh-9s/wiki/lost/HOST/OWNER/REPO/<file>.<timestamp>`), the status says so with the path, and the page is
  reloaded. If the remote moved but not this page, the commit goes on top of it. A push that is still refused (someone pushed
  in between) is handled the same way: recovery file, reload, nothing half done.
- **Safety of the clone:** only regular files are read (symlinks are listed as pages that cannot be shown), file names
  are checked to be a single path element inside the clone, files over 1 MB are not shown or edited.
- **Formats:** `.md`, `.markdown`, `.mdown`, `.mkdn` are rendered; every other page file is shown as plain text and edited as
  text. Files that are not page-like (no extension that GitHub treats as a page, or binary) are not listed.
- **Keys and names:** `6` and `:wiki`, `e`, `n`, `x`, `tab`/`shift+tab` for links, `o`, `R`. List columns as in goal 1.
- **Limits:** 500 pages listed, 1 MB per page; a title of at most 100 characters.

## Left to the builder

File and function names; list column widths; how the editor seam works for a page (a variant of `editorCmd` that returns
a message carrying the page identity); the git plumbing (`diff --numstat`, `hash-object`/`rev-parse HEAD:file`) used for
line counts and for the blob comparison; test fakes (a local bare repo standing in for the remote through a seam on the URL).

## Manual checks I cannot run

A real push to a wiki of a throwaway repo (edit, create, delete); a private repo's wiki; a wiki on an Enterprise host;
a wiki with other formats and with subfolders; two sessions editing the same page; a repo whose wiki has no page yet.
