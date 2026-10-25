package launch

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// fakeSpace records the stages the sequence calls and fails the one named in
// failAt.
type fakeSpace struct {
	calls  []string
	failAt string
	pasted string
}

func (f *fakeSpace) step(name string) error {
	f.calls = append(f.calls, name)
	if f.failAt == name {
		return errors.New(name + " broke")
	}
	return nil
}

func (f *fakeSpace) open(r Request) (string, error) { return "%1", f.step("open") }
func (f *fakeSpace) start(pane string, r Request) error {
	return f.step("start")
}
func (f *fakeSpace) ready(pane string, r Request, timeout time.Duration) error {
	return f.step("ready")
}
func (f *fakeSpace) paste(pane, text string) error {
	f.pasted = text
	return f.step("paste")
}

func TestSequenceOrder(t *testing.T) {
	f := &fakeSpace{}
	if err := runSequence(f, Request{Context: "hello"}, time.Second); err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "start", "ready", "paste"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	if f.pasted != "hello" {
		t.Errorf("pasted %q, want the request context", f.pasted)
	}
}

func TestSequenceStopsOnFailure(t *testing.T) {
	stages := []string{"open", "start", "ready", "paste"}
	steps := map[string]string{"open": StepOpen, "start": StepStart, "ready": StepReady, "paste": StepPaste}
	for i, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			f := &fakeSpace{failAt: stage}
			err := runSequence(f, Request{}, time.Second)
			var se *StepError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *StepError", err)
			}
			if se.Step != steps[stage] {
				t.Errorf("Step = %q, want %q", se.Step, steps[stage])
			}
			if want := stages[:i+1]; !reflect.DeepEqual(f.calls, want) {
				t.Errorf("calls = %v, want %v", f.calls, want)
			}
			if got, want := se.Error(), steps[stage]+": "+stage+" broke"; got != want {
				t.Errorf("Error() = %q, want %q", got, want)
			}
		})
	}
}

func TestReadyTimeoutPastesNothing(t *testing.T) {
	f := &fakeSpace{failAt: "ready"}
	err := runSequence(f, Request{Context: "secret"}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepReady {
		t.Fatalf("err = %v, want StepReady", err)
	}
	if f.pasted != "" {
		t.Errorf("pasted %q after a timeout, want nothing", f.pasted)
	}
}

func TestStepErrorUnwrap(t *testing.T) {
	inner := errors.New("boom")
	if err := error(&StepError{Step: StepFetch, Err: inner}); !errors.Is(err, inner) {
		t.Error("StepError does not unwrap to its cause")
	}
}
