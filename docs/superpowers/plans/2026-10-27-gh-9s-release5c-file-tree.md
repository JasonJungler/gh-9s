# Release 5c — the diff file tree with file-type icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The review screen's file list is a nested, foldable directory tree (single-child directory chains merged on one row), and with Nerd Font icons every folder and file shows an icon chosen by language/kind.

**Architecture:** `internal/icons` gains `FileIcon(path)` and `FolderIcon(open)`. A pure `internal/tui/filetree.go` turns the diff's files into tree rows (with aggregates and fold state); `review.go`'s `listLine`/`sideBody` and the unified `f` list draw from those rows. Fold state lives in `diffState`; the file-index based machinery (`starts`, `viewed`, `threads`) is unchanged.

**Tech Stack:** Go (mise), the icon table from release 5b.

**Spec:** `docs/superpowers/specs/2026-10-27-gh-9s-release5-design.md` ("Part 3").

## Global Constraints

- The tree is for everyone: plain mode gets the nesting (two columns of indentation per level, `name/` with a `▾` open / `▸` folded marker, files without an icon); only the glyphs differ between the sets.
- Rows are ordered directories first, then files, each alphabetically (case-insensitive, then exact); a directory with exactly one child that is a directory is merged with it (`internal/tui`); merging stops at a directory with more than one child or any file.
- Fold state is keyed by the merged directory's full path; folding is only a view: the diff pane, `]`/`[`, search and the file-index based data are unchanged.
- Aggregates on a directory row: added/removed totals, viewed `k/n` when any file under it is viewed, comment and draft counts under it. File rows keep what they show today (`+n −m`, `✓` viewed, `◆n`, `✎n`).
- Nerd file icons: single-column BMP private-use glyphs from Seti-UI/devicons in Nerd Font v3 (check each code point against the Nerd Fonts glyph names list), each with its language's usual colour; tested like the other icons (one rune, width 1, in `U+E000–U+F8FF`). Unknown extensions get a generic file icon; no icon in the plain set.
- Large PRs: a 3,000-file diff builds its rows once per fetch/fold change, not per frame; the list scrolls and keeps the selection visible.
- No test touches the user's config. Icons are process-wide: tests restore `plain`. Run Go as `mise exec -- go ...` with `export PATH=/opt/homebrew/bin:$PATH`. Branch `release5c-tree` off `main`; no push/tag without approval. README updated.

## Review Focus

- Odd paths: a file at the repository root next to directories; paths with spaces, unicode, a leading dot (`.github/workflows/ci.yml`), a file and a directory with the same prefix (`a` and `a.go`, `a/` and `a-b/`); a rename that moves a file between directories (new path used for placement, old path shown as `old → new` on the file row); a file whose name equals a directory's name elsewhere.
- Merging: `a/b/c/file` alone → one row `a/b/c`; adding a sibling file in `a` splits it (`a` → `b/c`); folding a merged row then the diff refetching with the same files keeps the fold, with different files drops folds that no longer exist.
- Selection: moving over rows with folds, folding the directory that holds the selection (selection moves to the directory), unfolding, `g/G`, a resize, the diff pane scrolling to another file while the list is focused or not.
- `v` on a directory with a mix of viewed and unviewed files (toggles to all viewed; all viewed → none) and with folded children; the head-commit reset of viewed marks still applies.
- Icons: `.tsx`, `.d.ts`, `Dockerfile.dev`, `docker-compose.yml`, `CMakeLists.txt`, `go.sum`, `.env`, uppercase extensions (`.PNG`), double extensions (`.test.js`, `.spec.ts`), no extension, a dotfile only, very long names truncated from the left without cutting the icon.
- Width: tree rows with CJK names, tabs, and 40–200 column terminals stay exactly the list width.

---

### Task 1: File and folder icons

**Files:**
- Create: `internal/icons/files.go`, `internal/icons/files_test.go`

**Interfaces:**
- Produces: `func FileIcon(path string) (glyph, color string)` and `func FolderIcon(open bool) (glyph, color string)`, both empty in the plain set; `var fileKinds` (extension → kind) and `var fileNames` (special file names → kind) tables inside the package.

- [ ] **Step 1: Write the failing tests:** table over paths → expected kind for at least: `main.go`, `a/b/app.py`, `x.js`, `x.jsx`, `x.ts`, `x.tsx`, `x.d.ts`, `a.test.js`, `a.spec.ts`, `lib.rs`, `A.java`, `a.c`, `a.h`, `a.cpp`, `a.cs`, `a.rb`, `a.php`, `a.swift`, `a.kt`, `a.sh`, `a.lua`, `README.md`, `a.json`, `a.yml`, `a.yaml`, `a.toml`, `a.html`, `a.css`, `a.scss`, `a.sql`, `Dockerfile`, `Dockerfile.dev`, `docker-compose.yml`, `Makefile`, `go.mod`, `go.sum`, `package.json`, `package-lock.json`, `.gitignore`, `.env`, `a.PNG`, `a.txt`, `LICENSE`, `noext`, `.hidden`, `weird.xyz`; each Nerd glyph is one BMP-private-use rune of width 1 and the colour is non-empty for known kinds; the same path in plain mode gives ""; `FolderIcon` open/closed differ; extension matching is case-insensitive and double extensions pick the more specific (`a.test.js` → test kind, `a.d.ts` → typescript definitions or typescript); every glyph in the tables is one column wide (iterate the tables).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/icons -run File` — Expected: FAIL.
- [ ] **Step 3: Implement** (code points from the Nerd Fonts glyph names list — Seti-UI `nf-seti-*`, devicons `nf-dev-*`, Font Awesome `nf-fa-*`; comment each with its name). **Step 4: Run** `mise exec -- go test ./internal/icons -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: file and folder icons by language`.

### Task 2: The tree rows (pure)

**Files:**
- Create: `internal/tui/filetree.go`, `internal/tui/filetree_test.go`

**Interfaces:**
- Produces:
  - `type treeRow struct { dir bool; path string /* full path; merged chain for dirs */; name string /* what the row shows: "internal/tui" or "file.go" or "old → new" */; depth int; file int /* index in diff.Files, -1 for dirs */; added, removed, files, viewed, threads, drafts int }`
  - `func buildTree(files []diff.File, viewed map[string]bool, threads []ghcli.Thread, drafts []drafts.Draft, folded map[string]bool) []treeRow` — the visible rows in display order

- [ ] **Step 1: Write the failing tests:** ordering (dirs first, case-insensitive, then exact; `a` vs `a.go` vs `a/`); merged chains and their split when a sibling appears; root-level files after directories; aggregates (added/removed, files, viewed, threads and drafts under a directory); folding hides descendants and keeps the directory row; fold state keyed by merged path; renames (placed by the new path, name `old → new`); a single file; an empty diff; unicode and space paths; 3,000 files build under 50 ms.
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run Tree` — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** the same — Expected: PASS.
- [ ] **Step 5: Commit** `feat: the diff files as a directory tree`.

### Task 3: Draw and drive the tree

**Files:**
- Modify: `internal/tui/review.go` (`listLine`, `sideBody`, list keys), `internal/tui/diffview.go` (`diffState.folded`, `rows []treeRow` cache, the unified `f` list), `README.md`
- Test: `internal/tui/filetree_view_test.go` (+ existing review tests adjusted where the list changed)

**Interfaces:**
- Consumes: Tasks 1–2. Produces: `diffState.folded map[string]bool` and a cached `treeRows` rebuilt when files, viewed marks, threads, drafts or folds change; `listLine` draws a `treeRow` (indent, fold marker or icon, name truncated from the left keeping the icon, aggregates); list keys: `j/k/g/G` move over rows, `enter` on a file jumps the diff, `enter`/`space` on a directory folds/unfolds, `v` toggles viewed for a file or every file under a directory, `h`/`l` fold/unfold the directory (and `h` on a file selects its parent).

- [ ] **Step 1: Write the failing tests:** side-by-side at 140 columns shows the nested rows (directory rows with `/`, files indented) in plain mode and with icons in nerd mode (folder + language file icons); `enter` on a directory folds it and its files leave the list while the diff pane keeps all files; folding the directory that holds the selection moves the selection to it; `v` on a directory with mixed marks marks all, again clears all, and marks persist across a refetch at the same head; the diff-focused list selection follows the file on top; the unified `f` full-screen list uses the same tree; truncation keeps icons and exact width (CJK, long names) at 40/60/140/200 columns; 3,000 files scroll; the old flat-list tests still pass or are replaced with tree equivalents (listed in the commit message).
- [ ] **Step 2: Run** `mise exec -- go test ./internal/tui -run 'Tree|Review|Diff'` — Expected: FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `mise exec -- sh -c 'gofmt -l .; go vet ./... && go test ./... -race -count=2'` — Expected: all `ok`.
- [ ] **Step 5: Mutation check** (skip merging single-child chains; make `v` on a directory mark only the first file; drop the icon from truncated names) and **commit** `feat: the review file list is a foldable tree`.
- [ ] **Step 6: Manual check by the user:** list in the final message what to try (a PR touching several directories: folding, merged rows, viewed on a directory; in a Nerd Font terminal look at the language icons for Go, TS, Markdown, YAML, Dockerfile, lock files).
