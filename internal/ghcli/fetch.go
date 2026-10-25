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

// FetchBase brings the remote that base names up to date with git fetch, so a new branch starts from what the
// remote has now. base is <remote>/<branch> or a bare branch (which means the remote called origin). When the
// repository has no remote to fetch from it is not an error: note says the local branch is used instead.
// A fetch that fails, or a base that names no remote when there is no origin to fall back on, is an error
// (carrying git's own message for a failed fetch), never a fallback.
func FetchBase(dir, base string) (note string, err error) {
	out, err := fetchGit(dir, "remote")
	if err != nil {
		return "", err
	}
	remotes := strings.Fields(out)
	remote := ""
	for _, r := range remotes {
		if strings.HasPrefix(base, r+"/") && len(r) > len(remote) {
			remote = r
		}
	}
	if remote == "" {
		for _, r := range remotes {
			if r == "origin" {
				remote = r
			}
		}
	}
	if len(remotes) == 0 {
		return noRemoteNote, nil
	}
	if remote == "" {
		return "", fmt.Errorf("base %q names no remote and there is no origin (remotes: %s)", base, strings.Join(remotes, ", "))
	}
	if _, err := fetchGit(dir, "fetch", remote); err != nil {
		return "", err
	}
	return "", nil
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
