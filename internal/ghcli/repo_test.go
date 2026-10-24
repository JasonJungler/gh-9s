package ghcli

import (
	"reflect"
	"testing"
)

func TestParseRepoRef(t *testing.T) {
	ok := map[string]RepoRef{
		"acme/api":              {Owner: "acme", Name: "api"},
		"ghe.corp.com/acme/api": {Host: "ghe.corp.com", Owner: "acme", Name: "api"},
	}
	for in, want := range ok {
		got, err := ParseRepoRef(in)
		if err != nil || got != want {
			t.Errorf("ParseRepoRef(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "bad", "a/b/c/d", "/x", "x/"} {
		if _, err := ParseRepoRef(bad); err == nil {
			t.Errorf("ParseRepoRef(%q) should fail", bad)
		}
	}
}

func TestRepoRefArgs(t *testing.T) {
	if got := (RepoRef{}).Args(); got != nil {
		t.Errorf("zero args = %v; want nil", got)
	}
	got := RepoRef{Owner: "acme", Name: "api"}.Args()
	if want := []string{"-R", "acme/api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v; want %v", got, want)
	}
	if s := (RepoRef{Host: "h.com", Owner: "o", Name: "n"}).String(); s != "h.com/o/n" {
		t.Errorf("String = %q", s)
	}
}
