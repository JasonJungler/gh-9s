package workflow

import (
	"reflect"
	"testing"
)

func TestYAMLAnchorsAreResolved(t *testing.T) {
	needs := `
x: &n [a]
jobs:
  a: {}
  b:
    needs: *n
`
	specs, err := ParseJobSpecs([]byte(needs))
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
	specs, _ = ParseJobSpecs([]byte(merged))
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
	in, ok, err := ParseDispatchInputs([]byte(inputs))
	if err != nil || !ok || len(in) != 2 || in[1].Type != "choice" || !reflect.DeepEqual(in[1].Options, []string{"x", "y"}) || in[1].Default != "x" {
		t.Errorf("aliased input: %+v ok=%v err=%v", in, ok, err)
	}
}
