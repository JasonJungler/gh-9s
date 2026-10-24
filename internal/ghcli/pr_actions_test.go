package ghcli

import (
	"reflect"
	"testing"
)

var acmeAPI = RepoRef{Owner: "acme", Name: "api"}

func TestReviewArgs(t *testing.T) {
	cases := []struct {
		name string
		r    RepoRef
		kind ReviewKind
		body string
		want []string
	}{
		{"approve without a body", acmeAPI, ReviewApprove, "", []string{"pr", "review", "482", "--approve", "-R", "acme/api"}},
		{"approve with a body", acmeAPI, ReviewApprove, "LGTM", []string{"pr", "review", "482", "--approve", "--body=LGTM", "-R", "acme/api"}},
		{"request changes", acmeAPI, ReviewRequestChanges, "Please add a test", []string{"pr", "review", "482", "--request-changes", "--body=Please add a test", "-R", "acme/api"}},
		{"comment", RepoRef{}, ReviewComment, "A thought", []string{"pr", "review", "482", "--comment", "--body=A thought"}},
		{"enterprise host", RepoRef{Host: "ghe.corp.com", Owner: "acme", Name: "api"}, ReviewApprove, "", []string{"pr", "review", "482", "--approve", "-R", "ghe.corp.com/acme/api"}},
	}
	for _, c := range cases {
		if got := ReviewArgs(c.r, "482", c.kind, c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}

func TestBodiesStayInOneArgumentWhateverTheyContain(t *testing.T) {
	for _, body := range []string{"-1 from me", "--approve; rm -rf ~", "multi\nline\ntext", "$(whoami) `id` \"quoted\"", "  spaces  "} {
		for _, args := range [][]string{
			ReviewArgs(acmeAPI, "1", ReviewComment, body),
			CommentArgs(acmeAPI, "1", body),
		} {
			found := false
			for _, a := range args {
				if a == "--body="+body {
					found = true
				}
			}
			if !found {
				t.Errorf("body %q was not kept as a single --body= argument: %q", body, args)
			}
		}
	}
}

func TestCommentArgs(t *testing.T) {
	got := CommentArgs(acmeAPI, "482", "Looks good")
	want := []string{"pr", "comment", "482", "--body=Looks good", "-R", "acme/api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}
