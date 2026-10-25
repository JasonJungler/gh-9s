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
	_, note, err := FetchBase(r.dir, "origin/main")
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
	_, note, err := FetchBase(r.dir, "main")
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
	if _, _, err := FetchBase(r.dir, "upstream/main"); err != nil {
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
	_, note, err := FetchBase(local, "origin/main")
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
	_, note, err := FetchBase(r.dir, "origin/main")
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

func TestFetchBaseUnmatchedRemoteNoOrigin(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "rename", "origin", "upstream")
	gitIn(t, r.dir, "remote", "add", "fork", r.bare)
	before := gitIn(t, r.dir, "rev-parse", "upstream/main")
	r.pushNew(t)
	for _, base := range []string{"main", "origin/main"} {
		_, note, err := FetchBase(r.dir, base)
		if err == nil {
			t.Fatalf("base %q: want an error, got note %q", base, note)
		}
		if note != "" {
			t.Errorf("base %q: note = %q, want none", base, note)
		}
		for _, want := range []string{base, "upstream", "fork"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("base %q: error %q does not mention %q", base, err, want)
			}
		}
	}
	if got := gitIn(t, r.dir, "rev-parse", "upstream/main"); got != before {
		t.Error("upstream was fetched although the base named no remote")
	}
	if out := gitIn(t, r.dir, "branch", "-r"); strings.Contains(out, "fork/") {
		t.Errorf("fork was fetched: %s", out)
	}
}

func TestFetchBaseBranchWithSlashes(t *testing.T) {
	r := newFetchRepos(t)
	want := r.pushNew(t)
	if _, _, err := FetchBase(r.dir, "origin/feature/x"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got != want {
		t.Errorf("origin/main = %s, want %s", got, want)
	}
}

func TestFetchBaseBareNameThatLooksLikeARemote(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "add", "fork", r.bare)
	gitIn(t, r.other, "push", "-q", "origin", "main:fork") // a branch on origin that is called like a remote
	want := r.pushNew(t)
	if ref, _, err := FetchBase(r.dir, "fork"); err != nil || ref != "origin/fork" {
		t.Fatalf("FetchBase = %q, %v; want origin/fork", ref, err)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got != want {
		t.Errorf("origin/main = %s, want %s", got, want)
	}
}

func TestDefaultBranchFromOriginHead(t *testing.T) {
	r := newFetchRepos(t)
	if got := DefaultBranch(r.dir); got != "main" {
		t.Errorf("DefaultBranch = %q, want main", got)
	}
	// a remote whose default branch is not main, with a slash in its name
	gitIn(t, r.dir, "checkout", "-q", "-b", "release/v2")
	gitIn(t, r.dir, "push", "-q", "origin", "release/v2")
	gitIn(t, r.dir, "remote", "set-head", "origin", "release/v2")
	if got := DefaultBranch(r.dir); got != "release/v2" {
		t.Errorf("DefaultBranch = %q, want release/v2", got)
	}
}

func TestDefaultBranchFallsBackToMain(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "set-head", "origin", "--delete")
	if got := DefaultBranch(r.dir); got != "main" {
		t.Errorf("without origin/HEAD: DefaultBranch = %q, want main", got)
	}
	lone := t.TempDir()
	gitIn(t, lone, "init", "-q", "-b", "trunk")
	if got := DefaultBranch(lone); got != "main" {
		t.Errorf("no remote: DefaultBranch = %q, want main", got)
	}
	if got := DefaultBranch(filepath.Join(t.TempDir(), "missing")); got != "main" {
		t.Errorf("not a repository: DefaultBranch = %q, want main", got)
	}
}

func TestFetchBaseReturnsTheResolvedRef(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "add", "upstream", r.bare)
	gitIn(t, r.other, "push", "-q", "origin", "main:feature/x") // feature/x exists on origin, not locally
	gitIn(t, r.dir, "branch", "-q", "release", "main")          // local only
	gitIn(t, r.dir, "tag", "v1.2.0")
	sha := gitIn(t, r.dir, "rev-parse", "HEAD")
	local := func(base string) string { return "origin has no " + base + "; using the local ref " + base }
	for _, tc := range []struct{ base, ref, note string }{
		{"main", "origin/main", ""},
		{"feature/x", "origin/feature/x", ""},
		{"origin/main", "origin/main", ""},
		{"upstream/main", "upstream/main", ""},
		{"origin/feature/x", "origin/feature/x", ""},
		{"release", "release", local("release")},
		{"v1.2.0", "v1.2.0", local("v1.2.0")},
		{sha, sha, local(sha)},
		{"refs/heads/release", "refs/heads/release", ""},
		{"refs/tags/v1.2.0", "refs/tags/v1.2.0", ""},
	} {
		// FetchBase first: it fetches, so feature/x is known afterwards, which Resolve then sees without the network
		for name, fn := range map[string]func(string, string) (string, string, error){"FetchBase": FetchBase, "ResolveBase": ResolveBase} {
			ref, note, err := fn(r.dir, tc.base)
			if err != nil || note != tc.note || ref != tc.ref {
				t.Errorf("%s(%q) = %q, %q, %v; want %q, %q", name, tc.base, ref, note, err, tc.ref, tc.note)
			}
		}
	}
}

func TestFetchBaseNotFoundIsAnError(t *testing.T) {
	r := newFetchRepos(t)
	for _, base := range []string{"nope", "feature/none", "v9.9.9", "deadbeef", "-x", "--help"} {
		for name, fn := range map[string]func(string, string) (string, string, error){"FetchBase": FetchBase, "ResolveBase": ResolveBase} {
			ref, note, err := fn(r.dir, base)
			if err == nil {
				t.Errorf("%s(%q) = %q, %q; want an error", name, base, ref, note)
				continue
			}
			if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), base) {
				t.Errorf("%s(%q): error %q does not say the base was not found", name, base, err)
			}
		}
	}
}

func TestFetchBaseFindsABranchOnlyTheFetchBrings(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.other, "push", "-q", "origin", "main:fresh")
	if _, _, err := ResolveBase(r.dir, "fresh"); err == nil {
		t.Error("ResolveBase does not fetch, so it cannot know a branch that is only on the remote")
	}
	ref, note, err := FetchBase(r.dir, "fresh")
	if err != nil || ref != "origin/fresh" || note != "" {
		t.Errorf("FetchBase = %q, %q, %v; want origin/fresh", ref, note, err)
	}
}

func TestFetchBasePrefersTheRemoteOverAStaleLocalBranch(t *testing.T) {
	r := newFetchRepos(t)
	r.pushNew(t)
	ref, note, err := FetchBase(r.dir, "main") // local main is behind origin/main
	if err != nil || ref != "origin/main" || note != "" {
		t.Errorf("FetchBase = %q, %q, %v", ref, note, err)
	}
}

func TestResolveBaseDoesNotFetch(t *testing.T) {
	r := newFetchRepos(t)
	before := gitIn(t, r.dir, "rev-parse", "origin/main")
	r.pushNew(t)
	ref, _, err := ResolveBase(r.dir, "main")
	if err != nil || ref != "origin/main" {
		t.Fatalf("ref = %q, err = %v", ref, err)
	}
	if got := gitIn(t, r.dir, "rev-parse", "origin/main"); got != before {
		t.Error("ResolveBase fetched")
	}
}

func TestResolveBaseWithoutRemoteKeepsTheName(t *testing.T) {
	lone := t.TempDir()
	gitIn(t, lone, "init", "-q", "-b", "main")
	for _, base := range []string{"main", "origin/main"} {
		ref, note, err := FetchBase(lone, base)
		if err != nil || ref != base || note != "no remote, using the local branch" {
			t.Errorf("FetchBase(%q) = %q, %q, %v", base, ref, note, err)
		}
	}
}

func TestResolveBaseErrors(t *testing.T) {
	r := newFetchRepos(t)
	gitIn(t, r.dir, "remote", "rename", "origin", "upstream")
	if ref, _, err := ResolveBase(r.dir, "main"); err == nil {
		t.Errorf("a bare base with no origin must fail, got %q", ref)
	}
}

func TestDefaultBaseWithOrigin(t *testing.T) {
	r := newFetchRepos(t)
	base, note := DefaultBase(r.dir)
	if base != "origin/main" || note != "" {
		t.Errorf("DefaultBase = %q, %q; want origin/main and no note", base, note)
	}
}

func TestDefaultBaseWithoutRemoteIsTheLocalBranch(t *testing.T) {
	lone := t.TempDir()
	gitIn(t, lone, "init", "-q", "-b", "trunk")
	base, note := DefaultBase(lone)
	if base != "trunk" || note != "no remote, using the local branch" {
		t.Errorf("DefaultBase = %q, %q; want trunk with the no-remote note (origin/main would not exist)", base, note)
	}
	// a detached HEAD has no branch name: fall back to main
	gitIn(t, lone, "commit", "-q", "--allow-empty", "-m", "x")
	gitIn(t, lone, "checkout", "-q", "--detach")
	if base, _ := DefaultBase(lone); base != "main" {
		t.Errorf("detached: DefaultBase = %q, want main", base)
	}
}
