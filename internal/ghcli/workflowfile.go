package ghcli

import (
	"net/url"
	"strings"
)

// FetchWorkflowFile returns the raw workflow file at path for the given ref.
func FetchWorkflowFile(r RepoRef, path, ref string) ([]byte, error) {
	endpoint, flags := r.APIEndpoint("/contents/" + strings.TrimPrefix(path, "/"))
	if ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	args := append([]string{"api", "-H", "Accept: application/vnd.github.raw"}, append(flags, endpoint)...)
	return Run(args...)
}
