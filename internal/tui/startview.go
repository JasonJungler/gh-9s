package tui

// startViews maps the values of the start_view setting to the views they open.
var startViews = map[string]viewKind{
	"pulls": viewPRs, "issues": viewIssues, "runs": viewRuns, "workflows": viewWorkflows,
	"discussions": viewDiscussions, "wiki": viewWiki, "inbox": viewInbox,
}

// startOn opens the view the start_view setting names. Every one of them but the inbox needs a
// current repo, which is only known once the lookup in Init answers; until then startPending lets
// that answer swap in the inbox.
func (m model) startOn() model {
	m.view = startViews[m.cfg().String("start_view")]
	m.startPending = m.view != viewInbox
	return m
}
