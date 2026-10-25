package ghcli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// GitHub has no API for wikis: a wiki is a git repository (OWNER/REPO.wiki.git) and a page is a file in it.
// This file reads that repository through a local copy kept in the user's cache directory; changing it
// lives in wikiwrite.go.

// Limits.
const (
	WikiMaxPages = 500              // pages listed
	WikiMaxBytes = 1 << 20          // a page larger than this is neither shown nor edited
	wikiTimeout  = 90 * time.Second // one git call
)

// WikiState says what was found at the wiki's address.
type WikiState int

const (
	WikiReady   WikiState = iota // there is a wiki repository
	WikiOff                      // the repo has its wiki turned off
	WikiNoPages                  // the wiki is on but has no page yet (GitHub makes the repository with the first page)
)

// WikiPage is one page file.
type WikiPage struct {
	Title, File, Blob, Author string
	Updated                   time.Time
	Link                      bool // a symbolic link: listed, never followed
	Plain                     bool // not Markdown: shown and edited as plain text
}

// WikiList is the pages of a wiki, Home first and then by title.
type WikiList struct {
	State WikiState
	Pages []WikiPage
	Total int // pages there are, when more than WikiMaxPages
}

// WikiContent is one page with its text, and the pages there are (for following links).
type WikiContent struct {
	Page  WikiPage
	Text  string
	Plain bool
	Pages []WikiPage
}

// Outside things the wiki code uses; tests replace them.
var (
	wikiRoot = func() string {
		d, err := os.UserCacheDir()
		if err != nil {
			d = os.TempDir()
		}
		return filepath.Join(d, "gh-9s", "wiki")
	}
	wikiURL = func(host, owner, name string) string {
		return "https://" + host + "/" + owner + "/" + name + ".wiki.git"
	}
	wikiLogin = func(host string) (string, error) {
		out, err := RunHost(host, "api", "user", "--jq", ".login")
		return strings.TrimSpace(string(out)), err
	}
	wikiRepoEnabled = func(r RepoRef) (bool, error) {
		var v struct {
			Enabled bool `json:"hasWikiEnabled"`
		}
		err := JSON(&v, Scoped(r, "repo", "view", "--json", "hasWikiEnabled")...)
		return v.Enabled, err
	}
)

// wikiMu keeps two wiki calls from using the same local copy at once.
var wikiMu sync.Mutex

var (
	namePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	hostPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]{1,5})?$`)
)

func hostOf(r RepoRef) string {
	if r.Host == "" {
		return "github.com"
	}
	return r.Host
}

// checkWikiRepo refuses a repo that could not be a real GitHub one: the names go into a URL and a path.
func checkWikiRepo(r RepoRef) error {
	if r.Owner == "" || r.Name == "" {
		return errors.New("no repository to show a wiki for")
	}
	if !hostPart.MatchString(hostOf(r)) || !namePart.MatchString(r.Owner) || !namePart.MatchString(r.Name) || r.Owner == ".." || r.Name == ".." {
		return fmt.Errorf("%q is not a repo gh-9s will fetch a wiki for", r.String())
	}
	return nil
}

func wikiDir(r RepoRef) string {
	return filepath.Join(wikiRoot(), strings.ReplaceAll(hostOf(r), ":", "_"), r.Owner, r.Name+".wiki")
}

// newWikiGit builds a git call that cannot ask on the terminal, drops every credential helper but gh's own
// (so a token only goes where gh would send it), and runs no hooks.
func newWikiGit(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := append([]string{
		"-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential",
		"-c", "core.hooksPath=/dev/null", "-c", "core.quotepath=false",
	}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	if dir == "" {
		dir = os.TempDir()
	}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "LC_ALL=C")
	return cmd
}

func wikiGitCmd(dir string, args ...string) *exec.Cmd {
	return newWikiGit(context.Background(), dir, args...)
}

// wikiGit runs git in dir and returns stdout; the error carries git's own words.
var wikiGit = func(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wikiTimeout)
	defer cancel()
	cmd := newWikiGit(ctx, dir, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("git did not answer within %s", wikiTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, errors.New(msg)
	}
	return out, nil
}

// wikiSync brings the local copy to what GitHub has: clone it the first time, otherwise fetch and move the
// copy to the remote's default branch, throwing away anything local. A copy that cannot be brought up to date is
// deleted and cloned again. branch is "" when the remote has no history yet.
func wikiSync(tg RepoRef) (dir, branch string, st WikiState, err error) {
	if err = checkWikiRepo(tg); err != nil {
		return
	}
	dir = wikiDir(tg)
	url := wikiURL(hostOf(tg), tg.Owner, tg.Name)
	if _, serr := os.Stat(filepath.Join(dir, ".git")); serr == nil {
		if branch, err = wikiRefresh(dir, url); err == nil {
			return dir, branch, WikiReady, nil
		}
		if rerr := os.RemoveAll(dir); rerr != nil {
			return dir, "", WikiReady, rerr
		}
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return
	}
	if _, cerr := wikiGit("", "clone", "--quiet", "--", url, dir); cerr != nil {
		_ = os.RemoveAll(dir)
		st, err = wikiCloneFailed(tg, cerr)
		return dir, "", st, err
	}
	branch = wikiDefaultBranch(dir)
	if branch == "" {
		return dir, "", WikiNoPages, nil
	}
	// A remote whose HEAD names a branch that does not exist leaves nothing checked out: put the branch there.
	if _, err = wikiGit(dir, "reset", "--hard", "--quiet", "origin/"+branch); err != nil {
		return dir, "", WikiReady, err
	}
	return dir, branch, WikiReady, nil
}

func wikiRefresh(dir, url string) (string, error) {
	if _, err := wikiGit(dir, "remote", "set-url", "origin", url); err != nil {
		return "", err
	}
	if _, err := wikiGit(dir, "fetch", "--quiet", "--prune", "origin"); err != nil {
		return "", err
	}
	branch := wikiDefaultBranch(dir)
	if branch == "" {
		return "", errors.New("the remote has no branch")
	}
	if _, err := wikiGit(dir, "reset", "--hard", "--quiet", "origin/"+branch); err != nil {
		return "", err
	}
	if _, err := wikiGit(dir, "clean", "-fdxq"); err != nil {
		return "", err
	}
	return branch, nil
}

// wikiDefaultBranch is the branch the remote's HEAD names, else master or main when one exists; "" for none.
func wikiDefaultBranch(dir string) string {
	if out, err := wikiGit(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		b := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
		if _, err := wikiGit(dir, "rev-parse", "--verify", "--quiet", "origin/"+b); err == nil {
			return b
		}
	}
	for _, b := range []string{"master", "main"} {
		if _, err := wikiGit(dir, "rev-parse", "--verify", "--quiet", "origin/"+b); err == nil {
			return b
		}
	}
	return ""
}

// wikiCloneFailed turns a failed clone into a state or an error in words.
func wikiCloneFailed(tg RepoRef, err error) (WikiState, error) {
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "not found") || strings.Contains(low, "does not exist"):
		on, lerr := wikiRepoEnabled(tg)
		if lerr != nil {
			return WikiReady, fmt.Errorf("could not tell whether %s has a wiki: %s", tg, CleanText(lerr.Error()))
		}
		if !on {
			return WikiOff, nil
		}
		return WikiNoPages, nil
	case strings.Contains(low, "authentication") || strings.Contains(low, "could not read") || strings.Contains(low, "terminal prompts disabled") ||
		strings.Contains(low, "invalid credentials") || strings.Contains(low, "403"):
		return WikiReady, fmt.Errorf("git could not sign in to %s; run: gh auth login --hostname %s", hostOf(tg), hostOf(tg))
	}
	return WikiReady, errors.New(firstLine(CleanText(err.Error())))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// wikiExts are the page formats GitHub knows; md ones are shown as Markdown, the rest as plain text.
var wikiExts = map[string]bool{
	".md": true, ".markdown": true, ".mdown": true, ".mkdn": true,
	".txt": false, ".textile": false, ".asciidoc": false, ".adoc": false, ".rst": false, ".org": false,
	".creole": false, ".mediawiki": false, ".wiki": false, ".pod": false, ".rdoc": false,
}

// wikiFileExt tells whether file is a page file (a top-level file in a known format, not special or hidden).
func wikiFileExt(file string) (ext string, markdown, ok bool) {
	if file == "" || strings.ContainsAny(file, "/\\\x00") || strings.HasPrefix(file, ".") || strings.HasPrefix(file, "_") || strings.HasPrefix(file, "-") {
		return "", false, false
	}
	ext = strings.ToLower(filepath.Ext(file))
	md, known := wikiExts[ext]
	if !known || len(file) == len(ext) {
		return "", false, false
	}
	return ext, md, true
}

// WikiTitle is a page's title: its file name without the format, dashes turned into spaces.
func WikiTitle(file string) string {
	if ext, _, ok := wikiFileExt(file); ok {
		file = file[:len(file)-len(ext)]
	}
	return CleanText(strings.ReplaceAll(file, "-", " "))
}

// WikiURL is the page's address in the browser; the wiki's front address for no file.
func WikiURL(tg RepoRef, file string) string {
	u := "https://" + hostOf(tg) + "/" + tg.Owner + "/" + tg.Name + "/wiki"
	if file == "" {
		return u
	}
	if ext, _, ok := wikiFileExt(file); ok {
		file = file[:len(file)-len(ext)]
	}
	return u + "/" + url.PathEscape(file)
}

// WikiFileForTitle makes the file name for a new page's title, or says why the title cannot be used.
func WikiFileForTitle(title string) (string, error) {
	for _, r := range title {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return "", fmt.Errorf("a title cannot contain %q", string(r))
		}
	}
	title = strings.Join(strings.Fields(title), " ")
	switch {
	case title == "":
		return "", errors.New("a page needs a title")
	case utf8.RuneCountInString(title) > 100:
		return "", errors.New("a title can have at most 100 characters")
	case strings.HasPrefix(title, ".") || strings.HasPrefix(title, "-") || strings.HasPrefix(title, "_"):
		return "", errors.New("a title cannot start with . - or _")
	case strings.Contains(title, "-"):
		return "", errors.New("use a space instead of - in a title (GitHub turns file name dashes into spaces)")
	}
	for _, r := range title {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return "", fmt.Errorf("a title cannot contain %q", string(r))
		}
	}
	return strings.ReplaceAll(title, " ", "-") + ".md", nil
}

// wikiEntry is one line of the wiki's file tree.
type wikiEntry struct {
	file, blob string
	size       int
	link       bool
}

// wikiTree lists the top-level files of the copy's current snapshot.
func wikiTree(dir string) ([]wikiEntry, error) {
	out, err := wikiGit(dir, "ls-tree", "-l", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	var es []wikiEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) < 4 || f[1] != "blob" {
			continue // folders and submodules are not pages
		}
		size, _ := strconv.Atoi(f[3])
		es = append(es, wikiEntry{file: path, blob: f[2], size: size, link: f[0] == "120000"})
	}
	return es, nil
}

// wikiPages picks the page files out of the tree: one per title (Markdown wins), Home first, then by title.
func wikiPages(es []wikiEntry) []WikiPage {
	byTitle := map[string]WikiPage{}
	for _, e := range es {
		_, md, ok := wikiFileExt(e.file)
		if !ok {
			continue
		}
		p := WikiPage{Title: WikiTitle(e.file), File: e.file, Blob: e.blob, Link: e.link, Plain: !md}
		key := strings.ToLower(p.Title)
		if old, dup := byTitle[key]; dup && (!old.Plain || p.Plain && old.File < p.File) {
			continue
		}
		byTitle[key] = p
	}
	ps := make([]WikiPage, 0, len(byTitle))
	for _, p := range byTitle {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		hi, hj := strings.EqualFold(ps[i].Title, "home"), strings.EqualFold(ps[j].Title, "home")
		if hi != hj {
			return hi
		}
		return strings.ToLower(ps[i].Title) < strings.ToLower(ps[j].Title)
	})
	return ps
}

// ListWiki brings the local copy up to date and lists the pages with who last changed each and when.
func ListWiki(tg RepoRef) (WikiList, error) {
	wikiMu.Lock()
	defer wikiMu.Unlock()
	dir, _, st, err := wikiSync(tg)
	if err != nil || st != WikiReady {
		return WikiList{State: st}, err
	}
	es, err := wikiTree(dir)
	if err != nil {
		return WikiList{}, err
	}
	ps := wikiPages(es)
	l := WikiList{State: WikiReady, Total: len(ps)}
	if len(ps) > WikiMaxPages {
		ps = ps[:WikiMaxPages]
	}
	wikiStamp(dir, ps)
	l.Pages = ps
	return l, nil
}

// wikiStamp fills in who changed each page last and when, from one pass over the history.
func wikiStamp(dir string, ps []WikiPage) {
	out, err := wikiGit(dir, "log", "--no-renames", "--max-count=5000", "--format=%x1e%ct%x1f%an", "--name-only")
	if err != nil {
		return
	}
	want := map[string]int{}
	for i, p := range ps {
		want[p.File] = i
	}
	seen := map[string]bool{}
	for _, chunk := range strings.Split(string(out), "\x1e") {
		head, files, _ := strings.Cut(chunk, "\n")
		ts, author, ok := strings.Cut(head, "\x1f")
		if !ok {
			continue
		}
		sec, _ := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
		for _, f := range strings.Split(files, "\n") {
			i, ok := want[strings.TrimSpace(f)]
			if !ok || seen[f] {
				continue
			}
			seen[f] = true
			ps[i].Updated, ps[i].Author = time.Unix(sec, 0), CleanText(author)
		}
	}
}

// ReadWiki brings the local copy up to date and reads one page. The text is read from the repository's own
// objects, never through a path on disk, and a symbolic link is not followed.
func ReadWiki(tg RepoRef, file string) (WikiContent, error) {
	wikiMu.Lock()
	defer wikiMu.Unlock()
	return readWikiLocked(tg, file)
}

func readWikiLocked(tg RepoRef, file string) (WikiContent, error) {
	if _, _, ok := wikiFileExt(file); !ok {
		return WikiContent{}, fmt.Errorf("%q is not a page", file)
	}
	dir, _, st, err := wikiSync(tg)
	if err != nil {
		return WikiContent{}, err
	}
	if st != WikiReady {
		return WikiContent{}, errors.New("this wiki has no pages")
	}
	es, err := wikiTree(dir)
	if err != nil {
		return WikiContent{}, err
	}
	pages := wikiPages(es)
	for _, e := range es {
		if e.file != file {
			continue
		}
		if e.link {
			return WikiContent{}, errors.New("this page is a symbolic link; gh-9s does not follow those")
		}
		if e.size > WikiMaxBytes {
			return WikiContent{}, errors.New("this page is larger than 1 MB; it is not shown here (o opens it in the browser)")
		}
		out, err := wikiGit(dir, "cat-file", "blob", e.blob)
		if err != nil {
			return WikiContent{}, err
		}
		if !utf8.Valid(out) || strings.IndexByte(string(out), 0) >= 0 {
			return WikiContent{}, errors.New("this page is not text")
		}
		_, md, _ := wikiFileExt(file)
		return WikiContent{Page: WikiPage{Title: WikiTitle(file), File: file, Blob: e.blob, Plain: !md}, Text: string(out), Plain: !md, Pages: pages}, nil
	}
	return WikiContent{}, fmt.Errorf("no page %s in the wiki", file)
}

// WikiLineChange counts the lines of next that old does not have (added) and the lines of old that next does not
// have (removed). Lines are matched by text, so a moved line counts as neither.
func WikiLineChange(old, next string) (added, removed int) {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
	}
	o, n := split(old), split(next)
	have := map[string]int{}
	for _, l := range n {
		have[l]++
	}
	matched := 0
	for _, l := range o {
		if have[l] > 0 {
			have[l]--
			matched++
		} else {
			removed++
		}
	}
	return len(n) - matched, removed
}
