package main

import (
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

type dispatchInput struct {
	Name, Description, Type, Default string
	Required                         bool
	Options                          []string
}

type jobSpec struct {
	ID, Name string
	Needs    []string
}

// mapGet returns the value node for key in a mapping node, or nil.
func mapGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func parseRoot(src []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return nil, nil
}

func scalars(n *yaml.Node) []string {
	switch {
	case n == nil:
		return nil
	case n.Kind == yaml.ScalarNode:
		return []string{n.Value}
	case n.Kind == yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			out = append(out, c.Value)
		}
		return out
	}
	return nil
}

// parseDispatchInputs reads on.workflow_dispatch.inputs in file order.
// dispatchable reports whether the workflow can be triggered by hand.
func parseDispatchInputs(src []byte) (inputs []dispatchInput, dispatchable bool, err error) {
	root, err := parseRoot(src)
	if err != nil {
		return nil, false, err
	}
	on := mapGet(root, "on")
	if on == nil {
		// yaml.v3 keeps "on" as a string key, but tolerate YAML 1.1 style booleans.
		on = mapGet(root, "true")
	}
	switch {
	case on == nil:
		return nil, false, nil
	case on.Kind == yaml.MappingNode:
		wd := mapGet(on, "workflow_dispatch")
		if wd == nil {
			return nil, false, nil
		}
		ins := mapGet(wd, "inputs")
		if ins != nil && ins.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(ins.Content); i += 2 {
				spec := ins.Content[i+1]
				in := dispatchInput{Name: ins.Content[i].Value, Type: "string"}
				if v := mapGet(spec, "description"); v != nil {
					in.Description = v.Value
				}
				if v := mapGet(spec, "type"); v != nil && v.Value != "" {
					in.Type = v.Value
				}
				if v := mapGet(spec, "default"); v != nil {
					in.Default = v.Value
				}
				if v := mapGet(spec, "required"); v != nil {
					in.Required = v.Value == "true"
				}
				in.Options = scalars(mapGet(spec, "options"))
				inputs = append(inputs, in)
			}
		}
		return inputs, true, nil
	}
	for _, ev := range scalars(on) {
		if ev == "workflow_dispatch" {
			return nil, true, nil
		}
	}
	return nil, false, nil
}

// parseJobSpecs lists jobs in file order with their needs.
func parseJobSpecs(src []byte) ([]jobSpec, error) {
	root, err := parseRoot(src)
	if err != nil {
		return nil, err
	}
	jobs := mapGet(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, nil
	}
	var out []jobSpec
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		id := jobs.Content[i].Value
		j := jobs.Content[i+1]
		s := jobSpec{ID: id, Name: id, Needs: scalars(mapGet(j, "needs"))}
		if v := mapGet(j, "name"); v != nil && v.Value != "" {
			s.Name = v.Value
		}
		out = append(out, s)
	}
	return out, nil
}

// fetchWorkflowFile returns the raw workflow file at path for the given ref.
func fetchWorkflowFile(r repoRef, path, ref string) ([]byte, error) {
	repo := "{owner}/{repo}"
	var args []string
	if r != (repoRef{}) {
		repo = r.Owner + "/" + r.Name
		if r.Host != "" {
			args = append(args, "--hostname", r.Host)
		}
	}
	endpoint := "repos/" + repo + "/contents/" + strings.TrimPrefix(path, "/")
	if ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	args = append([]string{"api", "-H", "Accept: application/vnd.github.raw"}, append(args, endpoint)...)
	return gh(args...)
}
