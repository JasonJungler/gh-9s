package ghcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fetchRepos is a bare remote, a clone of it with the remote called origin, and a second clone to push from.
type fetchRepos struct {
	bare, dir, other string
}

// newFetchRepos builds the three repos in a temp directory, with git's config pinned so the user's own does not matter.
func newFetchRepos(t *testing.T) fetchRepos {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Test Person\n\temail = test@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r := fetchRepos{bare: filepath.Join(root, "remote.git"), dir: filepath.Join(root, "clone"), other: filepath.Join(root, "other")}
	gitIn(t, root, "init", "-q", "--bare", "-b", "main", r.bare)
	gitIn(t, root, "clone", "-q", r.bare, r.dir)
	gitIn(t, r.dir, "checkout", "-q", "-B", "main")
	gitIn(t, r.dir, "commit", "-q", "--allow-empty", "-m", "first")
	gitIn(t, r.dir, "push", "-q", "origin", "main")
	gitIn(t, root, "clone", "-q", r.bare, r.other)
	return r
}

// pushNew makes a commit in the second clone, pushes it, and returns its id.
func (r fetchRepos) pushNew(t *testing.T) string {
	t.Helper()
	gitIn(t, r.other, "commit", "-q", "--allow-empty", "-m", "second")
	gitIn(t, r.other, "push", "-q", "origin", "main")
	return gitIn(t, r.other, "rev-parse", "HEAD")
}

func TestFetchBaseUpdates(t *testing.T) {
	r := newFetchRepos(t)
	want := r.pushNew(t)
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got == want {
		t.Fatal("the clone already has the new commit; the test proves nothing")
	}
	note, err := FetchBase(r.dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got != want {
		t.Errorf("origin/main = %s, want %s", got, want)
	}
}

func TestFetchBaseBareBranchUsesOrigin(t *testing.T) {
	r := newFetchRepos(t)
	want := r.pushNew(t)
	note, err := FetchBase(r.dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got != want {
		t.Errorf("origin/main = %s, want %s", got, want)
	}
}

func TestFetchBaseOtherRemote(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "add", "upstream", r.bare)
	want := r.pushNew(t)
	if _, err := FetchBase(r.dir, "upstream/main"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.dir, "rev-parse", "upstream/main"); got != want {
		t.Errorf("upstream/main = %s, want %s", got, want)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got == want {
		t.Error("origin was fetched too; only the named remote should be")
	}
}

func TestFetchBaseNoRemote(t *testing.T) {
	r := newFetchRepos(t)
	local := filepath.Join(filepath.Dir(r.dir), "local")
	gitIn(t, filepath.Dir(r.dir), "init", "-q", "-b", "main", local)
	note, err := FetchBase(local, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if note != "no remote, using the local branch" {
		t.Errorf("note = %q", note)
	}
}

func TestFetchBaseFailure(t *testing.T) {
	r := newFetchRepos(t)
	if err := os.RemoveAll(r.bare); err != nil {
		t.Fatal(err)
	}
	note, err := FetchBase(r.dir, "origin/main")
	if err == nil {
		t.Fatal("a fetch that failed must be an error, not a fallback")
	}
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
	if !strings.Contains(err.Error(), "remote.git") {
		t.Errorf("error %q does not carry git's message", err)
	}
}
