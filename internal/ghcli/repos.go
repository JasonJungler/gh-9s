package ghcli

import (
	"encoding/json"
	"sort"
	"strings"
)

// RepoInfo is a repo as shown in the picker.
type RepoInfo struct {
	Ref         RepoRef
	Description string
	Language    string
	Stars       int
	UpdatedAt   string
}

// ParseRepoInfos reads the JSON of `gh repo list` or `gh search repos`. host is put on every
// Ref ("" for gh's default). Entries whose name is malformed are skipped.
func ParseRepoInfos(b []byte, host string) ([]RepoInfo, error) {
	var raw []struct {
		// gh repo list
		NameWithOwner   string `json:"nameWithOwner"`
		PrimaryLanguage *struct {
			Name string `json:"name"`
		} `json:"primaryLanguage"`
		StargazerCount int `json:"stargazerCount"`
		// gh search repos
		FullName        string `json:"fullName"`
		Language        string `json:"language"`
		StargazersCount int    `json:"stargazersCount"`
		// both
		Description *string `json:"description"`
		UpdatedAt   string  `json:"updatedAt"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var out []RepoInfo
	for _, r := range raw {
		name := r.NameWithOwner
		if name == "" {
			name = r.FullName
		}
		ref, err := ParseRepoRef(name)
		if err != nil || ref.Host != "" {
			continue
		}
		ref.Host = host
		info := RepoInfo{Ref: ref, Language: r.Language, Stars: max(r.StargazerCount, r.StargazersCount), UpdatedAt: r.UpdatedAt}
		if r.PrimaryLanguage != nil {
			info.Language = r.PrimaryLanguage.Name
		}
		if r.Description != nil {
			info.Description = *r.Description
		}
		out = append(out, info)
	}
	return out, nil
}

const repoFields = "nameWithOwner,description,primaryLanguage,stargazerCount,updatedAt"

// ListAccessibleRepos lists the repos the logged-in user can reach through their own account
// and the organisations they belong to (up to 200 each), most recently updated first. An
// organisation that cannot be listed is skipped; the call fails only if the user's own list does.
func ListAccessibleRepos(host string) ([]RepoInfo, error) {
	out, err := RunHost(host, "repo", "list", "--limit", "200", "--json", repoFields)
	if err != nil {
		return nil, err
	}
	all, err := ParseRepoInfos(out, host)
	if err != nil {
		return nil, err
	}
	if orgs, err := RunHost(host, "api", "user/orgs", "--jq", ".[].login"); err == nil {
		for _, org := range strings.Fields(string(orgs)) {
			if !safeLogin(org) {
				continue
			}
			o, err := RunHost(host, "repo", "list", org, "--limit", "200", "--json", repoFields)
			if err != nil {
				continue
			}
			if more, err := ParseRepoInfos(o, host); err == nil {
				all = append(all, more...)
			}
		}
	}
	seen := map[RepoRef]bool{}
	uniq := all[:0]
	for _, r := range all {
		if !seen[r.Ref] {
			seen[r.Ref] = true
			uniq = append(uniq, r)
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool { return uniq[i].UpdatedAt > uniq[j].UpdatedAt })
	return uniq, nil
}

func safeLogin(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.ContainsAny(s, " /\t")
}

// SearchRepos searches all of GitHub. The query goes after "--" so it can never be read as a flag.
func SearchRepos(host, query string, limit int) ([]RepoInfo, error) {
	out, err := RunHost(host, "search", "repos", "--limit", itoa(limit),
		"--json", "fullName,description,language,stargazersCount,updatedAt", "--", query)
	if err != nil {
		return nil, err
	}
	return ParseRepoInfos(out, host)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// FilterRepos keeps the repos matching q (case-insensitive) and orders them: name prefix first,
// then name substring, then description-only; each tier keeps the input order. An empty query
// returns the list unchanged.
func FilterRepos(list []RepoInfo, q string) []RepoInfo {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return list
	}
	var tiers [3][]RepoInfo
	for _, r := range list {
		name := strings.ToLower(r.Ref.Name)
		full := strings.ToLower(r.Ref.Owner + "/" + r.Ref.Name)
		switch {
		case strings.HasPrefix(name, q) || strings.HasPrefix(full, q):
			tiers[0] = append(tiers[0], r)
		case strings.Contains(full, q):
			tiers[1] = append(tiers[1], r)
		case strings.Contains(strings.ToLower(r.Description), q):
			tiers[2] = append(tiers[2], r)
		}
	}
	return append(append(tiers[0], tiers[1]...), tiers[2]...)
}
