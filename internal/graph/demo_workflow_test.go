package graph

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ui"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

// The repo ships .github/workflows/graph-demo.yml to exercise gh-9s by hand. This test
// proves, offline, that gh-9s understands that very file: it parses the real workflow and
// builds the graph from job names exactly as GitHub names them in a run.

const demoPath = "../../.github/workflows/graph-demo.yml"

func demoRunJobs(failing bool) []Job {
	var jobs []Job
	add := func(name, concl string) {
		status := "completed"
		if concl == "" {
			status = "in_progress"
		}
		jobs = append(jobs, Job{ID: fmt.Sprint(len(jobs) + 1), Name: name, Status: status, Conclusion: concl})
	}
	add("Prepare", "success")
	add("lint", "success")
	for _, go_ := range []string{"1.22", "1.23", "1.24"} {
		for _, os_ := range []string{"linux", "macos"} {
			c := "success"
			if failing && go_ == "1.23" && os_ == "linux" {
				c = "failure"
			}
			add(fmt.Sprintf("Unit (go %s / %s)", go_, os_), c)
		}
	}
	add("Docs", "success")
	add("skipped-example", "skipped")
	for _, t := range []string{"linux-amd64", "linux-arm64", "darwin-arm64", "windows-amd64"} {
		add("build ("+t+")", "success")
	}
	add("Deploy staging", "success")
	for _, b := range []string{"chromium", "firefox", "webkit"} {
		add("E2E / "+b, "success")
	}
	add("smoke", "success")
	add("Release", "")
	add("Notify", "success")
	return jobs
}

func TestDemoWorkflowInputsMatchTheTriggerForm(t *testing.T) {
	src, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	in, ok, err := workflow.ParseDispatchInputs(src)
	if err != nil || !ok {
		t.Fatalf("dispatchable=%v err=%v", ok, err)
	}
	var got []string
	for _, i := range in {
		got = append(got, fmt.Sprintf("%s:%s:%v", i.Name, i.Type, i.Required))
	}
	want := "scenario:choice:false fail_fast:boolean:false step_seconds:number:false ticket:string:true note:string:false"
	if strings.Join(got, " ") != want {
		t.Errorf("inputs = %v\nwant     %s", got, want)
	}
	if in[0].Default != "all-green" || len(in[0].Options) != 3 || in[1].Default != "false" || in[3].Default != "DEMO-1" {
		t.Errorf("defaults/options wrong: %+v", in)
	}
}

func TestDemoWorkflowBuildsAMultiStageMatrixGraph(t *testing.T) {
	src, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	specs, err := workflow.ParseJobSpecs(src)
	if err != nil {
		t.Fatal(err)
	}
	g := Build(demoRunJobs(false), specs)
	if g.Flat {
		t.Fatalf("demo workflow fell back to a flat graph: %s", g.Reason)
	}
	byName := map[string]Node{}
	var names []string
	for _, n := range g.Nodes {
		byName[n.Name] = n
		names = append(names, n.Name)
	}
	t.Logf("nodes: %v", names)
	if len(g.Nodes) != 11 {
		t.Fatalf("nodes = %d (%v); want 11", len(g.Nodes), names)
	}
	// matrix jobs collapse into groups with per-status counts
	for name, want := range map[string]struct {
		members int
		label   string
	}{"Unit": {6, "6✓"}, "build": {4, "4✓"}, "E2E": {3, "3✓"}} {
		n, ok := byName[name]
		if !ok || len(n.Members) != want.members || n.Label != want.label {
			t.Errorf("group %q = %+v; want %d members labelled %q", name, n, want.members, want.label)
		}
	}
	if n := byName["skipped-example"]; n.State != ui.StOther {
		t.Errorf("skipped job state = %v; want the neutral 'other' state", n.State)
	}
	if n := byName["Release"]; n.State != ui.StRun {
		t.Errorf("Release should be shown running, got %v", n.State)
	}
	stages := g.stages()
	if len(stages) != 6 {
		t.Fatalf("stages = %d (%v); want 6", len(stages), stages)
	}
	// stage membership: Prepare+lint | Unit+Docs+skipped | build | Deploy | E2E+smoke | Release+Notify
	wantStages := [][]string{
		{"Prepare", "lint"}, {"Unit", "Docs", "skipped-example"}, {"build"},
		{"Deploy staging"}, {"E2E", "smoke"}, {"Release", "Notify"},
	}
	for i, st := range stages {
		var got []string
		for _, idx := range st {
			got = append(got, g.Nodes[idx].Name)
		}
		if strings.Join(got, ",") != strings.Join(wantStages[i], ",") {
			t.Errorf("stage %d = %v; want %v", i, got, wantStages[i])
		}
	}
}

func TestDemoWorkflowFailureScenarioShowsTheFailedMember(t *testing.T) {
	src, _ := os.ReadFile(demoPath)
	specs, _ := workflow.ParseJobSpecs(src)
	g := Build(demoRunJobs(true), specs)
	if g.Flat {
		t.Fatalf("flat: %s", g.Reason)
	}
	for _, n := range g.Nodes {
		if n.Name == "Unit" {
			if n.State != ui.StFail || n.Label != "5✓ 1✗" {
				t.Errorf("Unit = state %v label %q; want failed, '5✓ 1✗'", n.State, n.Label)
			}
			return
		}
	}
	t.Error("no Unit node")
}

func TestDemoWorkflowBoxesFitAWideTerminalAndFallBackOnANarrowOne(t *testing.T) {
	src, _ := os.ReadFile(demoPath)
	specs, _ := workflow.ParseJobSpecs(src)
	g := Build(demoRunJobs(false), specs)
	lines, used := Render(g, StyleBoxes, 0, 400, at(60))
	if used != StyleBoxes {
		t.Fatalf("boxes should fit 400 columns, got %v", used)
	}
	need := 0
	for _, l := range lines {
		need = max(need, utf8.RuneCountInString(l))
	}
	t.Logf("the demo graph needs %d columns in the boxes style", need)
	if _, used := Render(g, StyleBoxes, 0, need-1, at(60)); used != StyleTree {
		t.Errorf("one column too narrow should fall back to the tree, got %v", used)
	}
	if _, used := Render(g, StyleBoxes, 0, need, at(60)); used != StyleBoxes {
		t.Errorf("exactly wide enough should keep boxes, got %v", used)
	}
}
