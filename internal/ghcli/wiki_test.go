package ghcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var wikiAcme = RepoRef{Owner: "acme", Name: "api"}

// gitIn runs git in dir for the test setup, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// wikiRemote is a bare repo standing in for GitHub's wiki repo, with a work tree to make changes "from another person".
type wikiRemote struct {
	t          *testing.T
	bare, work string
}

// newWikiRemote makes the remote with the given files (name → text) on a branch called branch, and points the seams at it.
func newWikiRemote(t *testing.T, branch string, files map[string]string) *wikiRemote {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(root, "gitconfig")
	_ = os.WriteFile(cfg, []byte("[user]\n\tname = Remote Person\n\temail = remote@example.com\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r := &wikiRemote{t: t, bare: filepath.Join(root, "remote.git"), work: filepath.Join(root, "work")}
	gitIn(t, root, "init", "-q", "--bare", "-b", branch, r.bare)
	gitIn(t, root, "clone", "-q", r.bare, r.work)
	gitIn(t, r.work, "checkout", "-q", "-B", branch)
	if len(files) > 0 {
		r.write(files, "first pages")
	}
	oldRoot, oldURL, oldLogin, oldEnabled := wikiRoot, wikiURL, wikiLogin, wikiRepoEnabled
	wikiRoot = func() string { return filepath.Join(root, "cache") }
	wikiURL = func(host, owner, name string) string { return r.bare }
	wikiLogin = func(host string) (string, error) { return "", errors.New("no login in tests") }
	wikiRepoEnabled = func(RepoRef) (bool, error) { return true, nil }
	t.Cleanup(func() { wikiRoot, wikiURL, wikiLogin, wikiRepoEnabled = oldRoot, oldURL, oldLogin, oldEnabled })
	return r
}

// write adds or changes files (an empty text deletes) as one commit pushed to the remote.
func (r *wikiRemote) write(files map[string]string, msg string) {
	r.t.Helper()
	for n, s := range files {
		p := filepath.Join(r.work, n)
		if s == "" {
			gitIn(r.t, r.work, "rm", "-q", "--", n)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			r.t.Fatal(err)
		}
		gitIn(r.t, r.work, "add", "--", n)
	}
	gitIn(r.t, r.work, "commit", "-q", "-m", msg)
	gitIn(r.t, r.work, "push", "-q", "origin", "HEAD")
}

func (r *wikiRemote) show(branch, file string) string {
	r.t.Helper()
	return gitIn(r.t, r.bare, "show", branch+":"+file)
}

func (r *wikiRemote) lastMessage(branch string) string {
	return gitIn(r.t, r.bare, "log", "-1", "--format=%s", branch)
}

func titles(ps []WikiPage) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Title)
	}
	return out
}

func TestListWikiPages(t *testing.T) {
	r := newWikiRemote(t, "master", map[string]string{
		"Home.md": "# Home\n", "Color-schemes.md": "colours", "notes.txt": "plain", "_Sidebar.md": "side", "image.png": "x",
		"dir/Sub.md": "sub", "Foo.md": "md", "Foo.rst": "rst", "Bar.adoc": "a", "Bar.md": "m", ".hidden.md": "h", "README": "no extension",
	})
	_ = r
	l, err := ListWiki(wikiAcme)
	if err != nil || l.State != WikiReady {
		t.Fatalf("%+v %v", l, err)
	}
	got := strings.Join(titles(l.Pages), "|")
	if got != "Home|Bar|Color schemes|Foo|notes" {
		t.Fatalf("pages = %q (Home first, then alphabetical; no sidebar, folders, images, dotfiles or extensionless files)", got)
	}
	if l.Pages[1].File != "Bar.md" || l.Pages[3].File != "Foo.md" {
		t.Errorf("a title with two formats: the Markdown one wins, got %s and %s", l.Pages[1].File, l.Pages[3].File)
	}
	h := l.Pages[0]
	if h.Author != "Remote Person" || h.Updated.IsZero() || h.Blob == "" || h.File != "Home.md" {
		t.Errorf("home = %+v", h)
	}
}

func TestListWikiCapsAfterSorting(t *testing.T) {
	files := map[string]string{"Home.md": "h"}
	for i := 0; i < 520; i++ {
		files[fmt.Sprintf("P%03d.md", i)] = "x"
	}
	newWikiRemote(t, "main", files)
	l, err := ListWiki(wikiAcme)
	if err != nil || len(l.Pages) != WikiMaxPages || l.Total != len(files) || l.Pages[0].Title != "Home" {
		t.Fatalf("pages=%d total=%d err=%v first=%v", len(l.Pages), l.Total, err, l.Pages[0].Title)
	}
}

func TestListWikiOffAndNoPages(t *testing.T) {
	newWikiRemote(t, "main", nil)
	wikiURL = func(host, owner, name string) string { return filepath.Join(t.TempDir(), "missing.git") }
	wikiRepoEnabled = func(RepoRef) (bool, error) { return false, nil }
	l, err := ListWiki(wikiAcme)
	if err != nil || l.State != WikiOff {
		t.Fatalf("wiki off: %+v %v", l, err)
	}
	wikiRepoEnabled = func(RepoRef) (bool, error) { return true, nil }
	l, err = ListWiki(wikiAcme)
	if err != nil || l.State != WikiNoPages {
		t.Fatalf("no pages: %+v %v", l, err)
	}
	wikiRepoEnabled = func(RepoRef) (bool, error) { return false, errors.New("boom") }
	if _, err = ListWiki(wikiAcme); err == nil {
		t.Fatal("a failed lookup must be an error, not 'wiki is off'")
	}
}

func TestListWikiEmptyRemoteIsNoPages(t *testing.T) {
	newWikiRemote(t, "main", nil) // a remote with no commit at all
	l, err := ListWiki(wikiAcme)
	if err != nil || l.State != WikiNoPages {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestListWikiRefusesOddRepoNames(t *testing.T) {
	newWikiRemote(t, "main", map[string]string{"Home.md": "h"})
	for _, r := range []RepoRef{{Owner: "..", Name: "api"}, {Owner: "acme", Name: "../x"}, {Owner: "-o", Name: "api"}, {Owner: "a/b", Name: "c"}, {Host: "evil.com/x", Owner: "a", Name: "b"}, {Owner: "a", Name: ""}} {
		if _, err := ListWiki(r); err == nil {
			t.Errorf("%+v was accepted", r)
		}
	}
}

func TestWikiGitCommandIsLockedDown(t *testing.T) {
	cmd := wikiGitCmd(t.TempDir(), "fetch")
	args := strings.Join(cmd.Args, " ")
	for _, want := range []string{"credential.helper= ", "credential.helper=!gh auth git-credential", "core.hooksPath=/dev/null"} {
		if !strings.Contains(args+" ", want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), "GIT_TERMINAL_PROMPT=0") {
		t.Error("git may ask on the terminal")
	}
	if strings.Contains(args, "http.extraHeader") || strings.Contains(args, "token") {
		t.Error("a token on the command line")
	}
}

func TestReadWikiPage(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "# Home\n[[Other]]\n", "notes.txt": "plain\n", "Big.md": strings.Repeat("x", WikiMaxBytes+1), "Bin.md": "a\x00b"})
	c, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil || c.Text != "# Home\n[[Other]]\n" || c.Plain || c.Page.Title != "Home" || c.Page.Blob == "" || len(c.Pages) != 4 {
		t.Fatalf("%+v %v", c, err)
	}
	if c, err = ReadWiki(wikiAcme, "notes.txt"); err != nil || !c.Plain {
		t.Fatalf("notes %+v %v", c, err)
	}
	for _, f := range []string{"Big.md", "Bin.md", "Nope.md", "../x", "-x.md", "a/b.md"} {
		if _, err := ReadWiki(wikiAcme, f); err == nil {
			t.Errorf("%s was read", f)
		}
	}
	_ = r
}

func TestReadWikiSymlinkIsNotFollowed(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h"})
	if err := os.Symlink("/etc/hosts", filepath.Join(r.work, "Link.md")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.work, "add", "Link.md")
	gitIn(t, r.work, "commit", "-q", "-m", "link")
	gitIn(t, r.work, "push", "-q", "origin", "HEAD")
	l, err := ListWiki(wikiAcme)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range l.Pages {
		if p.File == "Link.md" {
			found = p.Link
		}
	}
	if !found {
		t.Errorf("the symlink should be listed as a link: %+v", l.Pages)
	}
	if _, err := ReadWiki(wikiAcme, "Link.md"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("err = %v", err)
	}
	var blob string
	for _, p := range l.Pages {
		if p.File == "Link.md" {
			blob = p.Blob
		}
	}
	if err := SaveWikiPage(wikiAcme, "Link.md", blob, "new", "m"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("saved through a symlink: %v", err)
	}
	if err := DeleteWikiPage(wikiAcme, "Link.md", blob, "m"); err != nil {
		t.Errorf("a link itself can be deleted: %v", err)
	}
}

func TestSaveWikiPagePushes(t *testing.T) {
	r := newWikiRemote(t, "master", map[string]string{"Home.md": "old\n", "Other.md": "o\n"})
	c, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "new text\n", "Update Home"); err != nil {
		t.Fatal(err)
	}
	if got := r.show("master", "Home.md"); got != "new text" {
		t.Errorf("remote has %q", got)
	}
	if r.lastMessage("master") != "Update Home" {
		t.Errorf("message %q", r.lastMessage("master"))
	}
	// the next read sees the saved text and a new blob
	c2, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil || c2.Text != "new text\n" || c2.Page.Blob == c.Page.Blob {
		t.Fatalf("%+v %v", c2, err)
	}
}

func TestSaveWikiPageOnTopOfOtherPages(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n", "Other.md": "o\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	r.write(map[string]string{"Other.md": "someone else\n"}, "other edit") // not this page
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "mine\n", "Update Home"); err != nil {
		t.Fatal(err)
	}
	if r.show("main", "Other.md") != "someone else" || r.show("main", "Home.md") != "mine" {
		t.Error("both edits must be there")
	}
}

func TestSaveWikiPageConflictKeepsTextAndWritesNothing(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	r.write(map[string]string{"Home.md": "theirs\n"}, "their edit")
	err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "mine\n", "Update Home")
	if err == nil || !strings.Contains(err.Error(), "changed on GitHub") {
		t.Fatalf("err = %v", err)
	}
	if r.show("main", "Home.md") != "theirs" {
		t.Error("their edit was overwritten")
	}
	path := recoveryPathIn(t, err)
	b, rerr := os.ReadFile(path)
	if rerr != nil || string(b) != "mine\n" {
		t.Fatalf("recovery file %s: %q %v", path, b, rerr)
	}
	if st, _ := os.Stat(path); st.Mode().Perm()&0o077 != 0 {
		t.Errorf("recovery file is readable by others: %v", st.Mode())
	}
}

// recoveryPathIn pulls the saved-text path out of an error message.
func recoveryPathIn(t *testing.T, err error) string {
	t.Helper()
	i := strings.Index(err.Error(), "saved in ")
	if i < 0 {
		t.Fatalf("no recovery path in %q", err)
	}
	return strings.TrimSpace(strings.SplitN(err.Error()[i+len("saved in "):], "\n", 2)[0])
}

func TestSaveWikiPageRejectedPushKeepsTextAndRecovers(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	hook := filepath.Join(r.bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'no write access' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "mine\n", "Update Home")
	if err == nil || !strings.Contains(err.Error(), "saved in ") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(recoveryPathIn(t, err)); string(b) != "mine\n" {
		t.Errorf("recovery text %q", b)
	}
	// the local copy is back in step with the remote, not left with the unpushed commit
	c2, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil || c2.Text != "old\n" {
		t.Fatalf("%+v %v", c2, err)
	}
	_ = os.Remove(hook)
	if err := SaveWikiPage(wikiAcme, "Home.md", c2.Page.Blob, "mine\n", "Update Home"); err != nil || r.show("main", "Home.md") != "mine" {
		t.Fatalf("a later save must work: %v", err)
	}
}

func TestSaveWikiPageRefusals(t *testing.T) {
	newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "old\n", "m"); err == nil || !strings.Contains(err.Error(), "no changes") {
		t.Errorf("unchanged: %v", err)
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "  \n", "m"); err == nil || !strings.Contains(err.Error(), "delete") {
		t.Errorf("emptied: %v", err)
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "x", " "); err == nil {
		t.Error("empty message")
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, strings.Repeat("x", WikiMaxBytes+1), "m"); err == nil {
		t.Error("too big")
	}
	if err := SaveWikiPage(wikiAcme, "Nope.md", "x", "x", "m"); err == nil {
		t.Error("missing page")
	}
	if err := SaveWikiPage(wikiAcme, "../Home.md", c.Page.Blob, "x", "m"); err == nil {
		t.Error("path escape")
	}
}

func TestCommitIdentityFallsBackToLoginNoreply(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty")) // no user.name / user.email
	t.Setenv("HOME", t.TempDir())
	var host string
	wikiLogin = func(h string) (string, error) { host = h; return "mira", nil }
	c, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "new\n", "Update Home"); err != nil {
		t.Fatal(err)
	}
	who := gitIn(t, r.bare, "log", "-1", "--format=%an <%ae>", "main")
	if who != "mira <mira@users.noreply.github.com>" || host != "github.com" {
		t.Errorf("author %q host %q", who, host)
	}
}

func TestCreateWikiPage(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n", "Color-schemes.md": "c"})
	if err := CreateWikiPage(wikiAcme, "Install-guide.md", "steps\n", "Create Install guide"); err != nil {
		t.Fatal(err)
	}
	if r.show("main", "Install-guide.md") != "steps" || r.lastMessage("main") != "Create Install guide" {
		t.Error("not pushed")
	}
	for _, f := range []string{"home.md", "COLOR-SCHEMES.md", "Home.txt", "Home.md"} {
		if err := CreateWikiPage(wikiAcme, f, "x", "m"); err == nil || !strings.Contains(err.Error(), "already") {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, f := range []string{"a/b.md", "../x.md", "-x.md", ".x.md", "x"} {
		if err := CreateWikiPage(wikiAcme, f, "x", "m"); err == nil {
			t.Errorf("%s was created", f)
		}
	}
	if err := CreateWikiPage(wikiAcme, "Empty.md", " \n", "m"); err == nil {
		t.Error("an empty page was created")
	}
}

func TestCreateWikiPageNeedsAWiki(t *testing.T) {
	newWikiRemote(t, "main", nil)
	if err := CreateWikiPage(wikiAcme, "First.md", "x", "m"); err == nil || !strings.Contains(err.Error(), "browser") {
		t.Errorf("err = %v", err)
	}
}

func TestDeleteWikiPage(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n", "Old.md": "o\n"})
	c, _ := ReadWiki(wikiAcme, "Old.md")
	r.write(map[string]string{"Old.md": "changed meanwhile\n"}, "edit")
	if err := DeleteWikiPage(wikiAcme, "Old.md", c.Page.Blob, "Delete Old"); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale delete: %v", err)
	}
	c, _ = ReadWiki(wikiAcme, "Old.md")
	if err := DeleteWikiPage(wikiAcme, "Old.md", c.Page.Blob, "Delete Old"); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", r.bare, "show", "main:Old.md").CombinedOutput(); err == nil {
		t.Errorf("still there: %s", out)
	}
	if r.lastMessage("main") != "Delete Old" {
		t.Errorf("message %q", r.lastMessage("main"))
	}
	if err := DeleteWikiPage(wikiAcme, "Old.md", c.Page.Blob, "Delete Old"); err == nil {
		t.Error("deleting a page that is gone")
	}
}

func TestDeleteLastPageLeavesNoPages(t *testing.T) {
	newWikiRemote(t, "main", map[string]string{"Home.md": "h\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	if err := DeleteWikiPage(wikiAcme, "Home.md", c.Page.Blob, "Delete Home"); err != nil {
		t.Fatal(err)
	}
	l, err := ListWiki(wikiAcme)
	if err != nil || l.State != WikiReady || len(l.Pages) != 0 {
		t.Fatalf("%+v %v", l, err)
	}
	if err := CreateWikiPage(wikiAcme, "Home.md", "again", "Create Home"); err != nil {
		t.Errorf("n must still work: %v", err)
	}
}

func TestDirtyOrBrokenCloneIsRepaired(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n"})
	if _, err := ListWiki(wikiAcme); err != nil {
		t.Fatal(err)
	}
	dir := wikiDir(wikiAcme)
	_ = os.WriteFile(filepath.Join(dir, "Home.md"), []byte("local junk"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Untracked.md"), []byte("junk"), 0o644)
	c, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil || c.Text != "h\n" {
		t.Fatalf("dirty: %+v %v", c, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Untracked.md")); err == nil {
		t.Error("untracked file survived")
	}
	_ = os.RemoveAll(filepath.Join(dir, ".git", "objects"))
	if c, err = ReadWiki(wikiAcme, "Home.md"); err != nil || c.Text != "h\n" {
		t.Fatalf("broken: %+v %v", c, err)
	}
	r.write(map[string]string{"Home.md": "h2\n"}, "edit")
	if c, err = ReadWiki(wikiAcme, "Home.md"); err != nil || c.Text != "h2\n" {
		t.Fatalf("updated: %+v %v", c, err)
	}
}

func TestWikiFileForTitle(t *testing.T) {
	ok := map[string]string{"Install guide": "Install-guide.md", "  Spaced   out ": "Spaced-out.md", "日本語": "日本語.md", "What's new": "What's-new.md"}
	for in, want := range ok {
		if got, err := WikiFileForTitle(in); err != nil || got != want {
			t.Errorf("%q → %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "  ", "A-B", "-x", ".x", "a/b", `a\b`, "a:b", "a*b", "a?b", `a"b`, "a<b", "a>b", "a|b", "a\x1bb", "a\nb", strings.Repeat("x", 101)} {
		if got, err := WikiFileForTitle(bad); err == nil {
			t.Errorf("%q was accepted as %q", bad, got)
		}
	}
}

func TestWikiTitleAndURL(t *testing.T) {
	if WikiTitle("Color-schemes.md") != "Color schemes" || WikiTitle("notes.txt") != "notes" || WikiTitle("Plain") != "Plain" {
		t.Error("title")
	}
	if got := WikiURL(RepoRef{Host: "ghe.example.com", Owner: "acme", Name: "api"}, "Color-schemes.md"); got != "https://ghe.example.com/acme/api/wiki/Color-schemes" {
		t.Error(got)
	}
	if got := WikiURL(wikiAcme, ""); got != "https://github.com/acme/api/wiki" {
		t.Error(got)
	}
}

func TestDiffStat(t *testing.T) {
	add, del := WikiLineChange("a\nb\nc\n", "a\nB\nc\nd\n")
	if add != 2 || del != 1 {
		t.Errorf("+%d -%d", add, del)
	}
	if add, del = WikiLineChange("a\n", "a\n"); add != 0 || del != 0 {
		t.Errorf("+%d -%d", add, del)
	}
	if add, del = WikiLineChange("", "x\ny\n"); add != 2 || del != 0 {
		t.Errorf("+%d -%d", add, del)
	}
}

func TestListWikiFindsTheBranchWhenRemoteHeadIsDangling(t *testing.T) {
	r := newWikiRemote(t, "main", nil) // the remote's HEAD names main ...
	gitIn(t, r.work, "checkout", "-q", "-b", "master")
	r.write(map[string]string{"Home.md": "h\n"}, "first") // ... but only master exists
	c, err := ReadWiki(wikiAcme, "Home.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "new\n", "m"); err != nil || r.show("master", "Home.md") != "new" {
		t.Fatalf("save to master: %v", err)
	}
}

func TestCloneFailureWords(t *testing.T) {
	newWikiRemote(t, "main", nil)
	notFound := errors.New("remote: Repository not found.\nfatal: repository 'https://github.com/acme/api.wiki.git/' not found")
	wikiRepoEnabled = func(RepoRef) (bool, error) { return false, nil }
	if st, err := wikiCloneFailed(wikiAcme, notFound); st != WikiOff || err != nil {
		t.Errorf("off: %v %v", st, err)
	}
	wikiRepoEnabled = func(RepoRef) (bool, error) { return true, nil }
	if st, err := wikiCloneFailed(wikiAcme, notFound); st != WikiNoPages || err != nil {
		t.Errorf("no pages: %v %v", st, err)
	}
	for _, msg := range []string{"fatal: could not read Username for 'https://ghe.example.com': terminal prompts disabled", "remote: Invalid username or password.\nfatal: Authentication failed", "The requested URL returned error: 403"} {
		_, err := wikiCloneFailed(RepoRef{Host: "ghe.example.com", Owner: "a", Name: "b"}, errors.New(msg))
		if err == nil || !strings.Contains(err.Error(), "gh auth login --hostname ghe.example.com") {
			t.Errorf("%q → %v", msg, err)
		}
	}
	if _, err := wikiCloneFailed(wikiAcme, errors.New("fatal: unable to access: Could not resolve host\x1b[31m: github.com\nmore")); err == nil || strings.ContainsRune(err.Error(), 0x1b) || strings.Contains(err.Error(), "more") {
		t.Errorf("generic error should be one clean line: %v", err)
	}
}

func TestEnterpriseHostGetsItsOwnNoreplyAddress(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty"))
	t.Setenv("HOME", t.TempDir())
	wikiLogin = func(h string) (string, error) { return "mira", nil }
	ghe := RepoRef{Host: "ghe.example.com", Owner: "acme", Name: "api"}
	c, err := ReadWiki(ghe, "Home.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveWikiPage(ghe, "Home.md", c.Page.Blob, "new\n", "m"); err != nil {
		t.Fatal(err)
	}
	if who := gitIn(t, r.bare, "log", "-1", "--format=%ae", "main"); who != "mira@users.noreply.ghe.example.com" {
		t.Errorf("email %q", who)
	}
}

func TestWikiReadFileHoldsNoWrites(t *testing.T) {
	b, err := os.ReadFile("wiki.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"commit", "push", "\"rm\"", "WriteFile"} {
		if strings.Contains(string(b), w) {
			t.Errorf("wiki.go holds %q; changes belong in wikiwrite.go", w)
		}
	}
}

// plumb adds a file to the test remote's work tree commit without touching the work tree (so symlinks and odd
// names can be stored on any file system).
func (r *wikiRemote) plumb(mode, name, content, msg string) {
	r.t.Helper()
	gitIn(r.t, r.work, "fetch", "-q", "origin")
	gitIn(r.t, r.work, "reset", "-q", "--hard", "@{upstream}")
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = r.work
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatal(err)
	}
	gitIn(r.t, r.work, "update-index", "--add", "--cacheinfo", mode+","+strings.TrimSpace(string(out))+","+name)
	gitIn(r.t, r.work, "commit", "-q", "-m", msg)
	gitIn(r.t, r.work, "push", "-q", "origin", "HEAD")
}

func TestDeletingAPageNamedLikeAGlobDeletesOnlyThatPage(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n", "Other.md": "o\n"})
	r.plumb("100644", "*.md", "star\n", "odd name")
	c, err := ReadWiki(wikiAcme, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteWikiPage(wikiAcme, "*.md", c.Page.Blob, "Delete *"); err != nil {
		t.Fatal(err)
	}
	if r.show("main", "Home.md") != "h" || r.show("main", "Other.md") != "o" {
		t.Error("deleting one page took others with it")
	}
	r.plumb("100644", ":!Home.md", "magic\n", "odd name 2")
	c, _ = ReadWiki(wikiAcme, ":!Home.md")
	if err := SaveWikiPage(wikiAcme, ":!Home.md", c.Page.Blob, "changed\n", "m"); err != nil || r.show("main", "Home.md") != "h" || r.show("main", ":!Home.md") != "changed" {
		t.Errorf("saving a page named like a pathspec: %v", err)
	}
}

func TestSaveNeverWritesThroughASymlinkOfAnotherCase(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n"})
	r.plumb("100644", "Foo.md", "foo\n", "foo")
	r.plumb("120000", "foo.md", "../outside.txt", "link that collides with Foo.md on a case-insensitive disk")
	outside := filepath.Join(filepath.Dir(wikiDir(wikiAcme)), "outside.txt")
	_ = os.MkdirAll(filepath.Dir(outside), 0o755)
	if err := os.WriteFile(outside, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ReadWiki(wikiAcme, "Foo.md")
	if err != nil {
		t.Fatal(err)
	}
	_ = SaveWikiPage(wikiAcme, "Foo.md", c.Page.Blob, "mine\n", "m") // may succeed or refuse; it must not escape
	if b, _ := os.ReadFile(outside); string(b) != "precious\n" {
		t.Fatalf("a file outside the clone was overwritten: %q", b)
	}
	if err := CreateWikiPage(wikiAcme, "New-one.md", "x\n", "m"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "precious\n" {
		t.Fatalf("a file outside the clone was overwritten by a new page: %q", b)
	}
}

func TestTextIsKeptWhateverTheFailure(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "old\n"})
	c, _ := ReadWiki(wikiAcme, "Home.md")
	good := wikiURL
	wikiURL = func(host, owner, name string) string { return filepath.Join(t.TempDir(), "gone.git") } // GitHub unreachable
	err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "my long edit\n", "m")
	if err == nil || !strings.Contains(err.Error(), "saved in ") {
		t.Fatalf("offline save: %v", err)
	}
	if b, _ := os.ReadFile(recoveryPathIn(t, err)); string(b) != "my long edit\n" {
		t.Errorf("kept %q", b)
	}
	wikiURL = good
	err = CreateWikiPage(wikiAcme, "Big.md", strings.Repeat("x", WikiMaxBytes+1), "m")
	if err == nil || !strings.Contains(err.Error(), "saved in ") {
		t.Errorf("too big: %v", err)
	}
	// nothing worth keeping: no changes, or an emptied page
	if err := SaveWikiPage(wikiAcme, "Home.md", c.Page.Blob, "old\n", "m"); err == nil || strings.Contains(err.Error(), "saved in ") {
		t.Errorf("no-change error should not leave a file: %v", err)
	}
	_ = r
}

func TestRecoveryFilesOfTheSameSecondDoNotOverwriteEachOther(t *testing.T) {
	newWikiRemote(t, "main", nil)
	a, b := wikiRecover(wikiAcme, "Home.md", "one"), wikiRecover(wikiAcme, "Home.md", "two")
	if a == "" || a == b {
		t.Fatalf("%q %q", a, b)
	}
	if x, _ := os.ReadFile(a); string(x) != "one" {
		t.Errorf("first text lost: %q", x)
	}
}

func TestKeepWikiText(t *testing.T) {
	newWikiRemote(t, "main", nil)
	p := KeepWikiText(wikiAcme, "Home.md", "unsent")
	if b, _ := os.ReadFile(p); p == "" || string(b) != "unsent" {
		t.Errorf("%q %q", p, b)
	}
}

func TestFileNamesWithControlCharactersAreNotPages(t *testing.T) {
	r := newWikiRemote(t, "main", map[string]string{"Home.md": "h\n"})
	r.plumb("100644", "bad\x1b]0;x\x07.md", "evil\n", "odd")
	l, err := ListWiki(wikiAcme)
	if err != nil || len(l.Pages) != 1 {
		t.Fatalf("%+v %v", l.Pages, err)
	}
	if _, err := ReadWiki(wikiAcme, "bad\x1b]0;x\x07.md"); err == nil || strings.ContainsRune(err.Error(), 0x1b) {
		t.Errorf("err %q", err)
	}
}
