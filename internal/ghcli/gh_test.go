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

func TestCleanLogStripsEscapeSequencesFromJobOutput(t *testing.T) {
	in := "build\tstep\t2026-01-01T00:00:00Z \x1b[31merror:\x1b[0m failed \x1b]0;title\x07done\nplain\n"
	got := CleanLog(in)
	want := []string{"step  error: failed done", "plain"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("CleanLog = %q; want %q (colours and terminal commands removed, the words kept)", got, want)
	}
}
