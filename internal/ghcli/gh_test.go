package ghcli

import (
	"reflect"
	"testing"
)

func TestCleanLog(t *testing.T) {
	in := "build\tSet up job\t2026-01-01T00:00:00.1234567Z hello\nbuild\tTest\t2026-01-01T00:00:01Z a\tb\nplain\n"
	want := []string{"Set up job  hello", "Test  a    b", "plain"}
	if got := CleanLog(in); !reflect.DeepEqual(got, want) {
		t.Errorf("CleanLog = %q; want %q", got, want)
	}
}
