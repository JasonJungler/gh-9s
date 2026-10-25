package ghcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// fetchTimeout bounds one git call made by FetchBase.
const fetchTimeout = 2 * time.Minute

// noRemoteNote is what FetchBase says when the repository has no remote at all.
const noRemoteNote = "no remote, using the local branch"

// ResolveBase says which ref base means, without touching the network (so it only knows what the repository
// has now). base is one of:
//   - <remote>/<branch> for a remote of the repository: kept as it is;
//   - a ref starting with refs/: kept as it is;
//   - a bare name. If origin has that branch (refs/remotes/origin/<name>), ref is "origin/<name>", so a new
//     branch starts from what the remote has and not from a stale local branch of the same name. Otherwise, if
//     the name is a local branch, a tag or a commit, ref is the name unchanged and note says origin has no such
//     branch and the local ref is used. Otherwise it is an error: the base was not found.
//
// When the repository has no remote at all, ref is base unchanged and note says the local branch is used. A
// bare name when there is no origin to fall back on is an error.
func ResolveBase(dir, base string) (ref, note string, err error) {
	_, ref, note, err = resolveBase(dir, base, false)
	return ref, note, err
}

// resolveBase is ResolveBase that also fetches the remote when fetch is set (before it looks for the bare name on
// origin), and returns the remote it fetched ("" when there is none).
func resolveBase(dir, base string, fetch bool) (remote, ref, note string, err error) {
	out, err := fetchGit(dir, "remote")
	if err != nil {
		return "", "", "", err
	}
	remotes := strings.Fields(out)
	if len(remotes) == 0 {
		return "", base, noRemoteNote, nil
	}
	if strings.HasPrefix(base, "refs/") {
		return "", base, "", nil
	}
	for _, r := range remotes {
		if strings.HasPrefix(base, r+"/") && len(r) > len(remote) {
			remote = r
		}
	}
	named := remote != ""
	if !named {
		for _, r := range remotes {
			if r == "origin" {
				remote = r
			}
		}
		if remote == "" {
			return "", "", "", fmt.Errorf("base %q names no remote and there is no origin (remotes: %s)", base, strings.Join(remotes, ", "))
		}
	}
	if fetch {
		if _, err := fetchGit(dir, "fetch", remote); err != nil {
			return "", "", "", err
		}
	}
	if named {
		return remote, base, "", nil
	}
	if hasCommit(dir, "refs/remotes/"+remote+"/"+base) {
		return remote, remote + "/" + base, "", nil
	}
	if !strings.HasPrefix(base, "-") && hasCommit(dir, base) {
		return remote, base, remote + " has no " + base + "; using the local ref " + base, nil
	}
	return "", "", "", fmt.Errorf("base %q not found: %s has no such branch, and it is no local branch, tag or commit", base, remote)
}

// hasCommit reports whether rev names a commit in the repository in dir.
func hasCommit(dir, rev string) bool {
	_, err := fetchGit(dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	return err == nil
}

// FetchBase resolves base like ResolveBase, after bringing that remote up to date with git fetch, so a new
// branch starts from what the remote has now (and a bare name is looked for on the remote as it is now). ref is
// what to branch from, note is what the user should know. A fetch that fails is an error (carrying git's own
// message), never a fallback.
func FetchBase(dir, base string) (ref, note string, err error) {
	_, ref, note, err = resolveBase(dir, base, true)
	return ref, note, err
}

// DefaultBase is the base the launch dialog starts with: origin's default branch as "origin/<branch>". A
// repository with no remote has no origin/<branch>, so it is the local branch that is checked out ("main" if
// none is), with the note that the local branch is used.
func DefaultBase(dir string) (base, note string) {
	out, err := fetchGit(dir, "remote")
	if err == nil && len(strings.Fields(out)) == 0 {
		b, err := fetchGit(dir, "symbolic-ref", "--short", "HEAD")
		if b = strings.TrimSpace(b); err != nil || b == "" {
			b = "main"
		}
		return b, noRemoteNote
	}
	return "origin/" + DefaultBranch(dir), ""
}

// DefaultBranch is the default branch of the origin remote of the repository in dir, as git last learned it
// (refs/remotes/origin/HEAD), without the "origin/" in front. It asks no server. When git does not know it (no
// origin, no origin/HEAD, not a repository) it is "main".
func DefaultBranch(dir string) string {
	out, err := fetchGit(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	b := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
	if b == "" {
		return "main"
	}
	return b
}

// fetchGit runs git in dir without ever asking on the terminal, and returns stdout; the error carries git's own words.
// It is not wikiGit: that one runs git with every credential helper but gh's dropped (it only talks to the wiki
// remote through gh), which would break fetching the user's own repository with their own helper or keychain.
func fetchGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "LC_ALL=C")
	cmd.WaitDelay = 5 * time.Second // a child that keeps stderr open cannot hold us past the timeout
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("git did not answer within " + fetchTimeout.String())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return string(out), nil
}
