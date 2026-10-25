# gh-9s: GitHub Discussions

Status: draft, waiting for approval. Nothing is built.

## Intent

The user wants to read a repo's Discussions and answer in them without leaving the terminal, as they already do for
issues and pull requests. In their words: view and comment; replies under a comment must be readable and writable
too; the list must be filterable by category. One release, reviewed by one fresh Opus reviewer, published only on the
user's explicit "發佈".

## Goals

1. **List** the current repo's Discussions (`:discussions`, alias `:disc`, number key `5` if free): columns
   `#`, `CATEGORY`, `TITLE`, `AUTHOR`, `COMMENTS`, `UPDATED`; a mark for answered (`✓`), closed and locked ones.
   Newest activity first (`UPDATED_AT` desc), first 50, and a note "first 50 of N" when there are more. The preview
   pane works like the issue list's (body of the selected Discussion).
2. **Filter by category:** `c` opens a choice dialog (`All` plus the repo's categories, with their emoji shortcode
   turned into nothing: the name only). The choice is part of the screen's identity and shown in the header
   (`category=Q&A`); it filters on the server (`categoryId`), so the first 50 are of that category.
3. **Read one Discussion** (`enter`): title, author, category, flags (answered / closed / locked), the body as
   Markdown (same renderer as issues, code blocks included), then the comments, each with its replies indented under it,
   oldest first. The comment marked as the answer shows `✓ answer`. Counts: up to 100 top-level comments and 50
   replies per comment are read; when there are more the screen says "N more not shown" at that place (and `o`
   opens the browser).
4. **Comment:** `C` adds a top-level comment. **Reply:** `r` replies to the selected comment (selecting a reply
   replies in the same thread: GitHub Discussions nest one level only). Both use the same dialog flow as the issue
   comment (a text dialog with the `ctrl+e` editor whose title names the repo and number, e.g. `acme/app#42`, and for
   a reply whom it answers; sending the dialog is the confirmation, as for an issue comment).
5. **Moving over comments:** `j`/`k` scroll as on the issue screen; `n`/`N` select the next / previous comment or
   reply (marked `▌` in the margin) and scroll it into view; `esc` goes back. (My call: `n`/`N` rather than `tab`,
   which the PR screen uses for images.)
6. `o` opens the Discussion (or the list) in the browser; `R` refreshes; `?` help and the footer list the keys.
7. Enterprise hosts work like everywhere else: the repo's host is used for every call (`gh api graphql --hostname`).

## Non-goals / anti-goals

Non-goals: creating, editing or deleting a Discussion or a comment, changing category, marking an answer, pinning,
locking or closing, voting, polls, labels, subscriptions, searching across repos, org-level Discussions, a demo GIF
(a follow-up).
Anti-goals: a comment or reply sent without the confirm; a reply that lands under the wrong comment because the
selection moved or the screen refreshed meanwhile; a late answer from a previous Discussion or category shown on the
current one; text from GitHub drawing escape sequences in the terminal.

## Constraints

Go 1.27 via mise (`mise exec -- go ...`), existing libraries only, only `internal/ghcli` runs `gh`. Follow the
codebase's rules: screens register in the registry; outside calls are package vars (seams) that tests replace;
identity (repo, number, category, comment id) is captured at key-press time and a stale async result is dropped by
identity; remote text goes through `ghcli.CleanText`; request bodies are JSON on stdin with ids as variables, never
inside the query text; every write confirms with the exact change in words; one write in flight; a timeout on every
call. The mutation lives only in a write file, and the existing test that restricts where mutations appear is
extended to it. Keys appear in `?` help, the footers, the README and the website's guide and key reference.

## What is known about GitHub's API (checked against GitHub's schema, not against a repo of the user's)

- `repository(owner,name){ hasDiscussionsEnabled discussionCategories(first:25){nodes{id name emoji isAnswerable}}
  discussions(first:50,after,categoryId,orderBy:{field:UPDATED_AT,direction:DESC}){totalCount pageInfo nodes{id number
  title url createdAt updatedAt closed locked isAnswered author{login} category{name emoji} comments{totalCount}}}}`
  answers on a public repo with Discussions (`cli/cli`); `MurabitoB/gh-9s` has them off.
- `repository{discussion(number:$n){ id title body url closed locked isAnswered answer{id} author category
  comments(first:100){totalCount pageInfo nodes{id body createdAt isAnswer author replies(first:50){totalCount
  pageInfo nodes{id body createdAt author}}}}}}` passes schema validation.
- `addDiscussionComment(input:{discussionId, body, replyToId})` passes schema validation; `replyToId` is the
  top-level comment's id (a reply to a reply must name the thread's top-level comment).
- Only `repo` scope is needed (no `project`-style extra scope).

## My calls (please check)

- Key `5` and `:disc` for the entry; `c` for category, `n`/`N` for moving over comments, `r` for reply.
- First 50 Discussions, 100 comments, 50 replies per comment, noted when cut off; no "load more".
- Locked Discussions refuse `C`/`r` with a status line instead of asking (the API would refuse anyway).
- Repos without Discussions show "Discussions are not enabled in acme/app" (from `hasDiscussionsEnabled`), not an error.
- Images in a Discussion body are not selectable (the issue screen's image viewer is not reused); a follow-up.
- Closed Discussions are listed (the API has no state filter) with a `closed` mark.
- Category emoji are dropped; only names are shown.
- The list query also reads `body` (the preview pane needs it); a body is cut to a sensible length for the list.
- After a comment or reply is sent, that Discussion is read again (same identity), so the new text shows.
- A reply's dialog names the author it answers. (There are no saved drafts: the issue comment dialog has none either; an earlier line of this spec said so by mistake.)

## Left to the builder

File and function names; the exact column widths; how the preview pane and body renderer are shared with the issue
screen; test fakes (a `discussionFakes` helper like `assignFakes`).

## Manual checks I cannot run

Writing to a real Discussion (use a throwaway repo with Discussions on); a Discussion with more than 100 comments;
a repo on an Enterprise host that has Discussions; a locked Discussion; a reply to a reply.
