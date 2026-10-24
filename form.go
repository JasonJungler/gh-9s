package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

type formField struct {
	In    dispatchInput
	Value string
}

// triggerForm is the workflow_dispatch form. Focus 0 is the ref picker,
// focus i (i >= 1) is fields[i-1].
type triggerForm struct {
	target        repoRef
	workflow      string
	ref           string
	loadedRef     string // the ref the current fields were read for
	refs          []string
	fields        []formField
	focus         int
	err           string
	pendingReload bool // the ref changed; the caller should re-read the inputs
}

func newTriggerForm(target repoRef, workflow, ref string, refs []string, inputs []dispatchInput) triggerForm {
	f := triggerForm{target: target, workflow: workflow, ref: ref, loadedRef: ref, refs: refs}
	return f.withInputs(inputs)
}

func initialValue(in dispatchInput) string {
	switch {
	case in.Default != "":
		return in.Default
	case in.Type == "boolean":
		return "false"
	case in.Type == "choice" && len(in.Options) > 0:
		return in.Options[0]
	}
	return ""
}

// withInputs replaces the fields, keeping values already typed for inputs
// that still exist.
func (f triggerForm) withInputs(inputs []dispatchInput) triggerForm {
	old := map[string]string{}
	for _, fl := range f.fields {
		old[fl.In.Name] = fl.Value
	}
	f.fields = nil
	for _, in := range inputs {
		v, ok := old[in.Name]
		if !ok {
			v = initialValue(in)
		}
		f.fields = append(f.fields, formField{In: in, Value: v})
	}
	if f.focus > len(f.fields) {
		f.focus = 0
	}
	return f
}

func (f triggerForm) stops() int { return len(f.fields) + 1 }

func (f *triggerForm) move(d int) {
	if f.focus == 0 && f.ref != f.loadedRef {
		f.pendingReload = true
	}
	f.focus = (f.focus + d + f.stops()) % f.stops()
}

func cycle(vals []string, cur string, d int) string {
	if len(vals) == 0 {
		return cur
	}
	i := -1
	for j, v := range vals {
		if v == cur {
			i = j
		}
	}
	return vals[((i+d)%len(vals)+len(vals))%len(vals)]
}

func (f triggerForm) update(k tea.KeyMsg) (next triggerForm, submit, cancel bool) {
	f.fields = append([]formField(nil), f.fields...)
	f.err = ""
	var fl *formField
	if f.focus > 0 {
		fl = &f.fields[f.focus-1]
	}
	switch k.Type {
	case tea.KeyEsc:
		return f, false, true
	case tea.KeyEnter:
		for _, x := range f.fields {
			if x.In.Required && strings.TrimSpace(x.Value) == "" {
				f.err = x.In.Name + " is required"
				return f, false, false
			}
		}
		return f, true, false
	case tea.KeyTab, tea.KeyDown:
		f.move(1)
	case tea.KeyShiftTab, tea.KeyUp:
		f.move(-1)
	case tea.KeyLeft, tea.KeyRight:
		d := 1
		if k.Type == tea.KeyLeft {
			d = -1
		}
		switch {
		case fl == nil:
			f.ref = cycle(f.refs, f.ref, d)
			f.pendingReload = f.ref != f.loadedRef
		case fl.In.Type == "choice":
			fl.Value = cycle(fl.In.Options, fl.Value, d)
		case fl.In.Type == "boolean":
			fl.Value = toggleBool(fl.Value)
		}
	case tea.KeySpace:
		switch {
		case fl != nil && fl.In.Type == "boolean":
			fl.Value = toggleBool(fl.Value)
		case fl == nil:
			f.ref += " "
		case fl.In.Type != "choice":
			fl.Value += " "
		}
	case tea.KeyBackspace:
		if fl == nil {
			f.ref = dropLast(f.ref)
		} else if fl.In.Type != "boolean" && fl.In.Type != "choice" {
			fl.Value = dropLast(fl.Value)
		}
	case tea.KeyRunes:
		switch {
		case fl == nil:
			f.ref += string(k.Runes)
		case fl.In.Type != "boolean" && fl.In.Type != "choice":
			fl.Value += string(k.Runes)
		}
	}
	return f, false, false
}

func toggleBool(v string) string {
	if v == "true" {
		return "false"
	}
	return "true"
}

func dropLast(s string) string {
	_, sz := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-sz]
}

// args is the `gh` argument list that dispatches the workflow. Empty optional
// text inputs are left out so the workflow's own defaults apply.
func (f triggerForm) args() []string {
	a := []string{"workflow", "run", f.workflow}
	a = append(a, f.target.args()...)
	a = append(a, "--ref", f.ref)
	for _, fl := range f.fields {
		if fl.Value == "" && !fl.In.Required {
			continue
		}
		a = append(a, "-f", fl.In.Name+"="+fl.Value)
	}
	return a
}

func (f triggerForm) view(width int) string {
	var b strings.Builder
	b.WriteString(styCols.Render("Trigger "+f.workflow) + "\n\n")
	row := func(i int, label, val string) {
		mark := "  "
		if f.focus == i {
			mark = styKey.Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%s%-18s %s\n", mark, label, val))
	}
	row(0, "ref", "‹ "+f.ref+" ›"+hint(f.focus == 0, "←/→ branch, or type"))
	for i, fl := range f.fields {
		label := fl.In.Name
		if fl.In.Required {
			label += "*"
		}
		var val string
		switch fl.In.Type {
		case "boolean":
			val = "[ ]"
			if fl.Value == "true" {
				val = "[x]"
			}
		case "choice":
			val = "‹ " + fl.Value + " ›"
		default:
			val = fl.Value
			if f.focus == i+1 {
				val += "█"
			}
		}
		row(i+1, label, val)
		if fl.In.Description != "" && f.focus == i+1 {
			b.WriteString(styDim.Render("                     "+fl.In.Description) + "\n")
		}
	}
	if f.err != "" {
		b.WriteString("\n" + styErr.Render(f.err) + "\n")
	}
	b.WriteString("\n" + styDim.Render("tab/↑↓ move · ←/→ choose · space toggle · enter run · esc cancel") + "\n")
	return b.String()
}

func hint(show bool, s string) string {
	if show {
		return "  " + styDim.Render(s)
	}
	return ""
}

// fetchBranches lists up to 100 branches of r with the default branch first.
func fetchBranches(r repoRef) ([]string, error) {
	ep, flags := r.apiEndpoint("")
	out, err := gh(append(append([]string{"api"}, flags...), ep, "-q", ".default_branch")...)
	if err != nil {
		return nil, err
	}
	def := strings.TrimSpace(string(out))
	ep, flags = r.apiEndpoint("/branches?per_page=100")
	out, err = gh(append(append([]string{"api"}, flags...), ep)...)
	if err != nil {
		return nil, err
	}
	var bs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &bs); err != nil {
		return nil, err
	}
	names := []string{def}
	for _, b := range bs {
		if b.Name != def {
			names = append(names, b.Name)
		}
	}
	return names, nil
}
