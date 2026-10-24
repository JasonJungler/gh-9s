package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShouldPoll(t *testing.T) {
	// a failed matrix group with a member still running must not stop the graph
	g := buildGraph([]runJob{job("build (a)", "completed", "failure"), job("build (b)", "in_progress", "")},
		[]jobSpec{{ID: "build", Name: "build"}})
	if g.nodes[0].State != stFail {
		t.Fatalf("setup: group state = %v", g.nodes[0].State)
	}
	m := model{view: viewGraph, graph: g, runStatus: "in_progress"}
	if !m.shouldPoll() {
		t.Error("graph of an in-progress run must keep polling even if a group shows failed")
	}
	m.runStatus = "completed"
	if m.shouldPoll() {
		t.Error("a completed run needs no polling")
	}
	m.runStatus = ""
	if !m.shouldPoll() {
		t.Error("unknown status (still loading) must poll")
	}
	if (model{view: viewFavs}).shouldPoll() {
		t.Error(":fav must not poll continuously")
	}
	if (model{view: viewLogs}).shouldPoll() || !(model{view: viewLogs, follow: true}).shouldPoll() {
		t.Error("logs poll only while following")
	}
	if !(model{view: viewRuns}).shouldPoll() {
		t.Error("runs list polls")
	}
}

func TestSpecsMapIsNotMutatedInPlace(t *testing.T) {
	orig := map[string][]jobSpec{}
	m := graphModel()
	m.specs = orig
	g := fixture()
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &g, jobs: nil,
		run: runMeta{"in_progress", ""}, specs: map[string][]jobSpec{"42": {{ID: "x", Name: "x"}}}})
	if len(orig) != 0 {
		t.Error("Update wrote into the map shared with in-flight fetch goroutines")
	}
	if got := next.(model).specs["42"]; len(got) != 1 {
		t.Errorf("new model lost the spec: %v", got)
	}
}

func TestFallbackReasonSurvivesKeypress(t *testing.T) {
	m := graphModel()
	flat := flatGraph([]runJob{ok("a")}, "dependencies unavailable")
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &flat, jobs: []runJob{ok("a")}, run: runMeta{"completed", "success"}})
	m = keys(next, "j")
	if !strings.Contains(m.View(), "dependencies unavailable") {
		t.Errorf("reason disappeared after a keypress:\n%s", m.View())
	}
}

func TestFavoritesDisabledMessageIsSticky(t *testing.T) {
	m := favModel(t)
	m.banner = "favorites disabled: bad json"
	m = keys(m, "j", "j")
	if !strings.Contains(m.View(), "favorites disabled") {
		t.Errorf("startup problem should stay visible:\n%s", m.View())
	}
}

func TestPagerCommandKeepsValuesOutOfTheShell(t *testing.T) {
	evil := repoRef{Owner: "a", Name: "b;touch /tmp/pwned"}
	cmd := pagerCommand(evil, "123", false)
	script := cmd.Args[2]
	if strings.Contains(script, "pwned") || strings.Contains(script, "123") {
		t.Errorf("values must be passed as arguments, not pasted into the script: %q", script)
	}
	if !reflect.DeepEqual(cmd.Args[4:], []string{"run", "view", "123", "--log", "-R", "a/b;touch /tmp/pwned"}) {
		t.Errorf("args = %q", cmd.Args[4:])
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
}

func TestLoadFavsRejectsHostileEntries(t *testing.T) {
	bad := []string{
		`[{"host":"github.com","repo":"a/b;rm -rf ~","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"-x/y","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"a/b","workflow":"../../user"}]`,
		`[{"host":"github.com","repo":"a/b","workflow":"--flag"}]`,
		`[{"host":"gh.com/evil","repo":"a/b","workflow":"x.yml"}]`,
		`[{"host":"github.com","repo":"nope","workflow":"x.yml"}]`,
	}
	for _, src := range bad {
		p := filepath.Join(t.TempDir(), "f.json")
		os.WriteFile(p, []byte(src), 0o644)
		if _, err := loadFavs(p); err == nil {
			t.Errorf("accepted %s", src)
		}
		if b, _ := os.ReadFile(p); string(b) != src {
			t.Errorf("file was modified for %s", src)
		}
	}
	good := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(good, []byte(`[{"host":"ghe.corp.com","repo":"acme/api-2","workflow":"deploy.yml","alias":"p"}]`), 0o644)
	if s, err := loadFavs(good); err != nil || len(s.items) != 1 {
		t.Errorf("valid entry rejected: %v", err)
	}
}

func TestYAMLAnchorsAreResolved(t *testing.T) {
	needs := `
x: &n [a]
jobs:
  a: {}
  b:
    needs: *n
`
	specs, err := parseJobSpecs([]byte(needs))
	if err != nil || len(specs) != 2 || !reflect.DeepEqual(specs[1].Needs, []string{"a"}) {
		t.Errorf("anchored needs: %+v %v", specs, err)
	}
	merged := `
defaults: &d
  needs: [a]
jobs:
  a: {}
  b:
    <<: *d
`
	specs, _ = parseJobSpecs([]byte(merged))
	if len(specs) != 2 || !reflect.DeepEqual(specs[1].Needs, []string{"a"}) {
		t.Errorf("merge key needs: %+v", specs)
	}
	inputs := `
on:
  workflow_dispatch:
    inputs:
      one: &common
        type: choice
        options: [x, y]
        default: x
      two: *common
`
	in, ok, err := parseDispatchInputs([]byte(inputs))
	if err != nil || !ok || len(in) != 2 || in[1].Type != "choice" || !reflect.DeepEqual(in[1].Options, []string{"x", "y"}) || in[1].Default != "x" {
		t.Errorf("aliased input: %+v ok=%v err=%v", in, ok, err)
	}
}
