package main

import "strings"

// searchMatches returns the indices of lines containing query. It is
// smart-case: an all-lowercase query ignores case, any capital makes it exact.
func searchMatches(lines []string, query string) []int {
	if query == "" {
		return nil
	}
	exact := query != strings.ToLower(query)
	if !exact {
		query = strings.ToLower(query)
	}
	var out []int
	for i, l := range lines {
		if !exact {
			l = strings.ToLower(l)
		}
		if strings.Contains(l, query) {
			out = append(out, i)
		}
	}
	return out
}
