package workflow

import (
	"reflect"
	"testing"
)

const dispatchYAML = `
name: Deploy
on:
  workflow_dispatch:
    inputs:
      env:
        description: Target
        type: choice
        options: [staging, prod]
        default: staging
        required: true
      dry:
        type: boolean
        default: true
      region:
        type: environment
      count:
        type: number
      note:
        description: free text
`

func TestParseDispatchInputs(t *testing.T) {
	in, ok, err := ParseDispatchInputs([]byte(dispatchYAML))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	var names, types []string
	for _, i := range in {
		names, types = append(names, i.Name), append(types, i.Type)
	}
	if want := []string{"env", "dry", "region", "count", "note"}; !reflect.DeepEqual(names, want) {
		t.Errorf("order = %v; want %v", names, want)
	}
	if want := []string{"choice", "boolean", "environment", "number", "string"}; !reflect.DeepEqual(types, want) {
		t.Errorf("types = %v; want %v", types, want)
	}
	if !reflect.DeepEqual(in[0].Options, []string{"staging", "prod"}) || in[0].Default != "staging" || !in[0].Required {
		t.Errorf("choice input = %+v", in[0])
	}
	if in[1].Default != "true" {
		t.Errorf("boolean default = %q; want \"true\"", in[1].Default)
	}
}

func TestDispatchableForms(t *testing.T) {
	cases := map[string]bool{
		"on: push\njobs: {}":                                    false,
		"on: workflow_dispatch\njobs: {}":                       true,
		"on: [push, workflow_dispatch]\njobs: {}":               true,
		"on:\n  push: {}\njobs: {}":                             false,
		"on:\n  workflow_dispatch:\njobs: {}":                   true,
		"on:\n  workflow_dispatch: {inputs: {a: {}}}\njobs: {}": true,
		"jobs: {}": false,
	}
	for src, want := range cases {
		_, got, err := ParseDispatchInputs([]byte(src))
		if err != nil || got != want {
			t.Errorf("%q: dispatchable=%v err=%v; want %v", src, got, err, want)
		}
	}
}

func TestParseJobSpecs(t *testing.T) {
	src := `
jobs:
  lint: {}
  test:
    name: Unit tests
  build:
    needs: [lint, test]
  ship:
    needs: build
  odd:
    needs: [ghost]
`
	got, err := ParseJobSpecs([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []JobSpec{
		{ID: "lint", Name: "lint"},
		{ID: "test", Name: "Unit tests"},
		{ID: "build", Name: "build", Needs: []string{"lint", "test"}},
		{ID: "ship", Name: "ship", Needs: []string{"build"}},
		{ID: "odd", Name: "odd", Needs: []string{"ghost"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestRequiredAcceptsEveryYAMLBoolean(t *testing.T) {
	for val, want := range map[string]bool{"true": true, "True": true, "TRUE": true, "false": false, "False": false} {
		src := "on:\n  workflow_dispatch:\n    inputs:\n      a:\n        required: " + val + "\n"
		in, _, err := ParseDispatchInputs([]byte(src))
		if err != nil || len(in) != 1 || in[0].Required != want {
			t.Errorf("required: %s -> %+v err=%v; want %v", val, in, err, want)
		}
	}
}
