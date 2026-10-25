package launch

import "time"

// Request is everything needed to launch an agent on an issue.
type Request struct {
	Target  Target
	Agent   Agent
	Model   string // empty means the agent's default
	Context string // the prompt text to paste once the agent is ready
	Dir     string // working directory for the new pane or window
	Place   Place
	Split   Split
	Base    string // base ref for an Orca workspace
	Name    string // branch or workspace name for an Orca workspace
	Issue   int

	// PasteOnly places Context in the agent's input box without submitting it: the user reviews it and
	// presses Enter in the agent. The zero value submits, as launches always have.
	PasteOnly bool

	// Nested makes an Orca workspace a child of the one gh-9s runs in (Orca's inferred parent). The zero
	// value makes it independent, with --no-parent. A retry must carry the same value as the launch it
	// repeats, because Orca asks for the exact same command.
	Nested bool

	// RetryID is the operation id of an Orca launch whose reply was lost (see
	// LostReplyError). When set, the same launch is asked again with it
	// instead of starting a new one.
	RetryID string
}

// Step names the stage of a launch that failed.
const (
	StepFetch  = "fetch"
	StepCreate = "create workspace"
	StepOpen   = "open pane"
	StepStart  = "start agent"
	StepReady  = "agent not ready"
	StepPaste  = "paste"
)

// StepError is a launch failure tagged with the stage it happened in.
type StepError struct {
	Step string
	Err  error
}

// Error returns "<Step>: <Err>".
func (e *StepError) Error() string { return e.Step + ": " + e.Err.Error() }

// Unwrap returns the underlying error.
func (e *StepError) Unwrap() error { return e.Err }

// space is a place an agent can run: it opens a pane, starts the agent in it,
// waits until the agent is ready and pastes the context into it.
type space interface {
	open(r Request) (pane string, err error)
	start(pane string, r Request) error
	ready(pane string, r Request, timeout time.Duration) error
	// paste puts text into the agent's input; with submit it also presses Enter.
	paste(pane, text string, submit bool) error
}

// runSequence opens a pane, starts the agent, waits for it to be ready and
// pastes r.Context (and submits it, unless r.PasteOnly). A space that can check the request first does so before
// anything is opened. It stops at the first failure and returns a *StepError.
// If the agent is not ready in time nothing is pasted and the pane is left
// open.
func runSequence(sp space, r Request, timeout time.Duration) error {
	if c, ok := sp.(interface{ check(Request) error }); ok {
		if err := c.check(r); err != nil {
			return &StepError{StepStart, err} // before open: nothing is left behind
		}
	}
	pane, err := sp.open(r)
	if err != nil {
		return &StepError{StepOpen, err}
	}
	if err := sp.start(pane, r); err != nil {
		return &StepError{StepStart, err}
	}
	if err := sp.ready(pane, r, timeout); err != nil {
		return &StepError{StepReady, err}
	}
	if err := sp.paste(pane, r.Context, !r.PasteOnly); err != nil {
		return &StepError{StepPaste, err}
	}
	return nil
}
