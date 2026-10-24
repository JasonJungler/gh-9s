package ghcli

import (
	"encoding/json"
	"strings"
)

// FetchBranches lists up to 100 branches of r with the default branch first.
func FetchBranches(r RepoRef) ([]string, error) {
	ep, flags := r.APIEndpoint("")
	out, err := Run(append(append([]string{"api"}, flags...), ep, "-q", ".default_branch")...)
	if err != nil {
		return nil, err
	}
	def := strings.TrimSpace(string(out))
	ep, flags = r.APIEndpoint("/branches?per_page=100")
	out, err = Run(append(append([]string{"api"}, flags...), ep)...)
	if err != nil {
		return nil, err
	}
	var bs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &bs); err != nil {
		return nil, err
	}
	names := []string{def}
	for _, b := range bs {
		if b.Name != def {
			names = append(names, b.Name)
		}
	}
	return names, nil
}
