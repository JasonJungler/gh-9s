package launch

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// RunFunc runs a command in dir with the given stdin and returns its standard
// output. It is injectable so tests never start real processes.
type RunFunc func(dir string, stdin []byte, name string, args ...string) ([]byte, error)

// ExecRun is the RunFunc backed by os/exec. When the command fails and wrote
// to standard error, the returned error's text is that stderr, trimmed.
func ExecRun(dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.Bytes(), errors.New(msg)
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}
