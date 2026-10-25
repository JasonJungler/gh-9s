package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
	"github.com/MurabitoB/gh-9s/internal/settings"
)

// launchFakes records what the launch seams were asked to do.
type launchFakes struct {
	env       map[string]string
	reqs      []launch.Request
	results   []launch.Result // answer i to request i; the last one repeats
	errs      []error
	modelsFor []string // agents whose models were listed
	ran       [][]string
	fetched   []string // issue numbers fetched for the list's a
}

const launchWD = "/work/app"

// installLaunch replaces every outside call of the launch with a fake. Nothing is run and no real
// environment variable is read.
func installLaunch(t *testing.T, env map[string]string) *launchFakes {
	t.Helper()
	f := &launchFakes{env: env}
	oldDo, oldModels, oldDir, oldBranch, oldEnv, oldL := launchDo, launchModels, launchDir, launchDefaultBranch, envGet, launcher
	launchDo = func(r launch.Request) (launch.Result, error) {
		f.reqs = append(f.reqs, r)
		i := len(f.reqs) - 1
		var res launch.Result
		var err error
		if len(f.results) > 0 {
			res = f.results[min(i, len(f.results)-1)]
		}
		if len(f.errs) > 0 {
			err = f.errs[min(i, len(f.errs)-1)]
		}
		return res, err
	}
	launchModels = func(a launch.Agent, run launch.RunFunc) []launch.Model {
		f.modelsFor = append(f.modelsFor, a.ID)
		if a.ID == "devin" {
			return []launch.Model{{}, {ID: "swe"}, {ID: "gemini"}}
		}
		return []launch.Model{{}, {ID: "opus"}, {ID: "sonnet"}}
	}
	launchDir = func() (string, error) { return launchWD, nil }
	launchDefaultBranch = func(string) string { return "trunk" }
	envGet = func(k string) string { return f.env[k] }
	launcher.Run = func(dir string, stdin []byte, name string, args ...string) ([]byte, error) {
		f.ran = append(f.ran, append([]string{name}, args...))
		return nil, errors.New("not in orca")
	}
	t.Cleanup(func() {
		launchDo, launchModels, launchDir, launchDefaultBranch, envGet, launcher = oldDo, oldModels, oldDir, oldBranch, oldEnv, oldL
	})
	return f
}

var tmuxEnv = map[string]string{"TMUX": "/tmp/tmux-1/default,1,0", "TMUX_PANE": "%1"}

// launchStores is a settings store and launch prefs in temp files, so nothing touches the real ones.
func launchStores(t *testing.T, kv ...string) (*settings.Store, *launch.Prefs, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := settings.Load(filepath.Join(dir, "settings.json"), func(string) string { return "" }, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if _, err := s.Set(kv[i], kv[i+1]); err != nil {
			t.Fatal(err)
		}
	}
	prefsPath := filepath.Join(dir, "launch.json")
	p, err := launch.LoadPrefs(prefsPath)
	if err != nil {
		t.Fatal(err)
	}
	return s, p, prefsPath
}

func issue42() ghcli.IssueDetail {
	return ghcli.IssueDetail{
		Number: 42, Title: "Fix login crash", State: "OPEN", URL: "https://github.com/acme/api/issues/42",
		Body: "It crashes.", Comments: []ghcli.Comment{{Author: "dana", Body: "same here"}},
	}
}

// issueOnScreen is the detail screen of issue42 with its data loaded, in the current directory's repo.
func issueOnScreen(t *testing.T, kv ...string) model {
	t.Helper()
	s, p, _ := launchStores(t, kv...)
	m := model{view: viewIssue, width: 130, height: 30, target: acme, curRepo: acme, issue: "42",
		settings: s, launchPrefs: p, previewOff: true}
	return m.withScreenData(viewIssue, issueState{d: issue42()})
}

// launchRun runs cmd and everything it leads to, batches included, feeding each message to m.
func launchRun(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			m = launchRun(m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	next, c := m.Update(msg)
	return launchRun(next.(model), c)
}

// press sends keys one by one and runs what they start.
func pressLaunch(m model, keys ...string) model {
	for _, key := range keys {
		next, c := m.Update(k(key))
		m = launchRun(next.(model), c)
	}
	return m
}

func TestAKeyOnDetailOpensDialog(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	m := pressLaunch(issueOnScreen(t), "a")
	if m.launchDlg == nil {
		t.Fatalf("a opened no dialog; status %q", ansi.Strip(m.status))
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Send #42", "Fix login crash", "tmux ◆ new pane (right) · claude · default model", "enter launch"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if len(f.modelsFor) != 1 || f.modelsFor[0] != "claude" {
		t.Errorf("models listed for %v; want claude once", f.modelsFor)
	}
	if got := m.launchDlg.in.ContextLen; got != len([]rune(launch.BuildContext(issue42()))) {
		t.Errorf("ContextLen = %d; want the rune count of the context", got)
	}
	if len(f.reqs) != 0 {
		t.Error("opening the dialog launched something")
	}
	if !strings.Contains(issueFooter(), "a agent") {
		t.Errorf("issue footer %q lacks a agent", issueFooter())
	}

	// not loaded yet: say so instead of opening on an empty issue
	m = issueOnScreen(t)
	m = m.withScreenData(viewIssue, issueState{})
	if m = pressLaunch(m, "a"); m.launchDlg != nil || !strings.Contains(ansi.Strip(m.status), "still loading the issue") {
		t.Errorf("a on an unloaded issue: dlg=%v status=%q", m.launchDlg != nil, ansi.Strip(m.status))
	}
}

func issueFooter() string  { return screens[viewIssue].footer }
func issuesFooter() string { return screens[viewIssues].footer }

func TestAKeyOnListFetchesIssueFirst(t *testing.T) {
	installLaunch(t, tmuxEnv)
	fi := installIssues(t, issue42())
	m := issuesLoaded(t)
	s, p, _ := launchStores(t)
	m.settings, m.launchPrefs, m.curRepo = s, p, acme
	fi.viewTargets = nil
	next, cmd := m.Update(k("a"))
	m = next.(model)
	if m.launchDlg != nil {
		t.Fatal("the dialog opened before the issue was fetched")
	}
	if cmd == nil {
		t.Fatal("a on a row started no fetch")
	}
	m = launchRun(m, cmd)
	if len(fi.viewTargets) != 1 || fi.viewTargets[0] != acme {
		t.Errorf("issue fetched for %v; want acme once", fi.viewTargets)
	}
	if m.launchDlg == nil || m.launchDlg.in.Issue.Number != 42 {
		t.Fatalf("no dialog for the fetched issue: %+v", m.launchDlg)
	}
	if !strings.Contains(issuesFooter(), "a agent") {
		t.Errorf("issues footer %q lacks a agent", issuesFooter())
	}

	// a failed fetch is a status line, not a dialog
	fetchIssue = func(ghcli.RepoRef, string) (ghcli.IssueDetail, error) {
		return ghcli.IssueDetail{}, errors.New("HTTP 404")
	}
	m.launchDlg = nil
	if m = pressLaunch(m, "a"); m.launchDlg != nil || !strings.Contains(ansi.Strip(m.status), "HTTP 404") {
		t.Errorf("failed fetch: dlg=%v status=%q", m.launchDlg != nil, ansi.Strip(m.status))
	}
}

func TestRefusesOtherRepo(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	fi := installIssues(t, issue42())
	other := ghcli.RepoRef{Host: "github.com", Owner: "acme", Name: "web"}

	m := issueOnScreen(t)
	m.curRepo = other
	m = pressLaunch(m, "a")
	if m.launchDlg != nil || !strings.Contains(ansi.Strip(m.status), "current directory") {
		t.Errorf("detail of another repo: dlg=%v status=%q", m.launchDlg != nil, ansi.Strip(m.status))
	}

	l := issuesLoaded(t)
	l.curRepo = other
	fi.viewTargets = nil
	l = pressLaunch(l, "a")
	if l.launchDlg != nil || !strings.Contains(ansi.Strip(l.status), "current directory") {
		t.Errorf("list of another repo: dlg=%v status=%q", l.launchDlg != nil, ansi.Strip(l.status))
	}
	if len(fi.viewTargets) != 0 || len(f.reqs) != 0 || len(f.modelsFor) != 0 || len(f.ran) != 0 {
		t.Errorf("something ran for another repo: fetch=%v launch=%v models=%v run=%v", fi.viewTargets, f.reqs, f.modelsFor, f.ran)
	}

	// the same repo written with the host is the same repo
	m = issueOnScreen(t)
	m.curRepo = ghcli.RepoRef{Host: "github.com", Owner: "Acme", Name: "API"}
	if m = pressLaunch(m, "a"); m.launchDlg == nil {
		t.Errorf("the current directory's repo was refused: %q", ansi.Strip(m.status))
	}
}

func TestNoEnvironmentMessage(t *testing.T) {
	f := installLaunch(t, map[string]string{})
	m := pressLaunch(issueOnScreen(t), "a")
	if m.launchDlg != nil || !strings.Contains(ansi.Strip(m.status), launch.ErrNoEnvironment.Error()) {
		t.Errorf("no environment: dlg=%v status=%q", m.launchDlg != nil, ansi.Strip(m.status))
	}
	if len(f.modelsFor)+len(f.reqs)+len(f.ran) != 0 {
		t.Error("something ran without an environment")
	}

	// an unknown launch_target in the settings file is dropped by the store (auto is used), so the
	// environment decides and nothing breaks
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"launch_target": "screen"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := settings.Load(path, func(string) string { return "" }, "")
	m = issueOnScreen(t)
	m.settings = s
	if m = pressLaunch(m, "a"); m.launchDlg != nil || !strings.Contains(ansi.Strip(m.status), launch.ErrNoEnvironment.Error()) {
		t.Errorf("unknown setting, no environment: dlg=%v status=%q", m.launchDlg != nil, ansi.Strip(m.status))
	}

	// an unknown override reaching Detect is shown, not acted on
	if _, why := issueOnScreen(t).launchRefusal("screen"); !strings.Contains(ansi.Strip(why), `unknown launch target "screen"`) {
		t.Errorf("unknown override: refusal %q", ansi.Strip(why))
	}
}

func TestEnterRunsLauncherOnceAndClosesOnSuccess(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	f.results = []launch.Result{{Where: "tmux pane", Note: "pasted 120 characters"}}
	m := issueOnScreen(t)
	prefsPath := filepath.Join(t.TempDir(), "launch.json")
	m.launchPrefs, _ = launch.LoadPrefs(prefsPath)
	// pick devin and its swe model in the form
	m = pressLaunch(m, "a", "e", "tab", "right", "tab", "right")
	if m.launchDlg == nil || m.launchDlg.in.Agent != "devin" || m.launchDlg.in.Model != "swe" {
		t.Fatalf("form did not pick devin/swe: %+v", m.launchDlg)
	}
	next, cmd := m.Update(k("enter"))
	m = next.(model)
	if m.launchDlg == nil || !m.launchDlg.running() {
		t.Fatal("enter did not mark the dialog as running")
	}
	if !strings.Contains(ansi.Strip(m.View()), "launching…") {
		t.Error("the running dialog does not say launching…")
	}
	if next, c := m.Update(k("enter")); c != nil || len(f.reqs) != 0 {
		t.Error("a second enter while running did something")
	} else {
		m = next.(model)
	}
	m = launchRun(m, cmd)
	if len(f.reqs) != 1 {
		t.Fatalf("launcher ran %d times; want once", len(f.reqs))
	}
	r := f.reqs[0]
	if r.Target != launch.TargetTmux || r.Agent.ID != "devin" || r.Model != "swe" || r.Dir != launchWD || r.Issue != 42 ||
		r.Context != launch.BuildContext(issue42()) || r.RetryID != "" {
		t.Errorf("request = %+v", r)
	}
	if m.launchDlg != nil {
		t.Error("the dialog stayed open after a success")
	}
	st := ansi.Strip(m.status)
	if !strings.Contains(st, "launched: tmux pane") || !strings.Contains(st, "pasted 120 characters") {
		t.Errorf("status = %q", st)
	}
	if got := m.cfg().String("launch_agent"); got != "devin" {
		t.Errorf("launch_agent = %q; want devin", got)
	}
	if got := m.launchPrefs.Model("devin"); got != "swe" {
		t.Errorf("remembered model = %q; want swe", got)
	}
	if b, err := os.ReadFile(prefsPath); err != nil || !strings.Contains(string(b), "swe") {
		t.Errorf("prefs file = %q, %v; want the model saved", b, err)
	}

	// the next dialog starts from the remembered agent and model
	m = pressLaunch(m, "a")
	if m.launchDlg == nil || m.launchDlg.in.Agent != "devin" || m.launchDlg.in.Model != "swe" {
		t.Errorf("next dialog = %+v; want devin/swe", m.launchDlg)
	}

	// Orca: the branch from the result is shown
	f.reqs = nil
	f.results = []launch.Result{{Where: "orca workspace issue-42-fix-login-crash", Branch: "me/issue-42-fix-login-crash"}}
	f.env = map[string]string{"ORCA_WORKTREE_ID": "x"}
	m = issueOnScreen(t)
	m = pressLaunch(m, "a", "enter")
	if len(f.reqs) != 1 || f.reqs[0].Base != "origin/trunk" || f.reqs[0].Name != "issue-42-fix-login-crash" {
		t.Fatalf("orca request = %+v", f.reqs)
	}
	if st := ansi.Strip(m.status); !strings.Contains(st, "launched: orca workspace issue-42-fix-login-crash me/issue-42-fix-login-crash") {
		t.Errorf("orca status = %q", st)
	}
}

func TestEscClosesWithoutLaunching(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	m := pressLaunch(issueOnScreen(t), "a", "e", "esc")
	if m.launchDlg != nil || len(f.reqs) != 0 {
		t.Errorf("esc: dlg=%v launches=%d", m.launchDlg != nil, len(f.reqs))
	}
}

func TestFailureKeepsDialogWithStep(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	f.errs = []error{&launch.StepError{Step: launch.StepReady, Err: errors.New("no ready marker after 30s")}, nil}
	f.results = []launch.Result{{}, {Where: "tmux pane"}}
	m := pressLaunch(issueOnScreen(t), "a", "enter")
	if m.launchDlg == nil {
		t.Fatalf("the dialog closed on a failure; status %q", ansi.Strip(m.status))
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "failed at agent not ready: no ready marker after 30s") || !strings.Contains(v, "enter to try again") {
		t.Errorf("view lacks the failed step:\n%s", v)
	}
	if m.cfg().Source("launch_agent") != settings.FromDefault {
		t.Error("a failed launch remembered the agent")
	}
	// enter again tries again, as a new launch
	m = pressLaunch(m, "enter")
	if len(f.reqs) != 2 || f.reqs[1].RetryID != "" || m.launchDlg != nil {
		t.Errorf("retry: launches=%d retry=%q dlg=%v", len(f.reqs), f.reqs[len(f.reqs)-1].RetryID, m.launchDlg != nil)
	}
}

func TestRetryReusesOperationId(t *testing.T) {
	f := installLaunch(t, map[string]string{"ORCA_WORKTREE_ID": "x"})
	lost := &launch.StepError{Step: launch.StepCreate, Err: &launch.LostReplyError{OperationID: "1730000000000-0123456789abcdef0123456789abcdef", Cause: "orca did not answer"}}
	f.errs = []error{lost}
	m := pressLaunch(issueOnScreen(t), "a", "enter")
	if m.launchDlg == nil {
		t.Fatal("the dialog closed on a lost reply")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "enter retries the same launch") {
		t.Errorf("view does not offer the retry:\n%s", v)
	}
	m = pressLaunch(m, "enter")
	if len(f.reqs) != 2 || f.reqs[1].RetryID != "1730000000000-0123456789abcdef0123456789abcdef" {
		t.Fatalf("second enter: %+v; want the operation id", f.reqs)
	}
	// an edit of any field makes it a new launch
	m = pressLaunch(m, "e", "tab", "tab", "tab", "tab", "x", "enter")
	if len(f.reqs) != 3 || f.reqs[2].RetryID != "" || f.reqs[2].Name != "issue-42-fix-login-crashx" {
		t.Errorf("after an edit: %+v; want a new launch", f.reqs[len(f.reqs)-1])
	}
}

func TestSettingsSupplyDefaults(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	m := issueOnScreen(t, "launch_tmux_place", "window", "launch_split", "vertical", "launch_target", "tmux")
	m = pressLaunch(m, "a")
	if m.launchDlg == nil {
		t.Fatalf("no dialog: %q", ansi.Strip(m.status))
	}
	if m.launchDlg.in.AutoTarget {
		t.Error("a target chosen in the settings is marked as detected")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "tmux · new window") {
		t.Errorf("summary does not show the window:\n%s", v)
	}
	m = pressLaunch(m, "enter")
	if len(f.reqs) != 1 || f.reqs[0].Place != launch.PlaceWindow || f.reqs[0].Split != launch.SplitVertical {
		t.Errorf("request = %+v; want window / vertical", f.reqs)
	}

	// launch_target chooses the environment over detection
	f.reqs = nil
	m = issueOnScreen(t, "launch_target", "herdr")
	if m = pressLaunch(m, "a", "enter"); len(f.reqs) != 1 || f.reqs[0].Target != launch.TargetHerdr {
		t.Errorf("request = %+v; want herdr", f.reqs)
	}
}

func TestDialogSwallowsKeys(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	m := pressLaunch(issueOnScreen(t), "a")
	for _, key := range []string{"q", "4", "x", "C", "?", ":", "a", "o", "/"} {
		next, cmd := m.Update(k(key))
		nm := next.(model)
		if cmd != nil || nm.view != viewIssue || nm.dlg != nil || nm.helpOpen || nm.mode != modeNormal || nm.launchDlg == nil {
			t.Errorf("%q acted behind the dialog: view=%v dlg=%v help=%v mode=%v cmd=%v", key, nm.view, nm.dlg != nil, nm.helpOpen, nm.mode, cmd != nil)
		}
		m = nm
	}
	if len(f.modelsFor) != 1 {
		t.Errorf("a second a started another dialog: models listed %v", f.modelsFor)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("ctrl+c does not quit while the dialog is open")
	}
}

func TestWheelDoesNotScrollThePaneBehindTheLaunchDialog(t *testing.T) {
	f := newPreviewFixture(t, 10)
	p := fullPR()
	for i := 0; i < 80; i++ {
		p.Body += "more\n\n"
	}
	f.settle(f.move(1))
	f.m.preview.put(f.m.preview.sel, previewEntry{pr: &p, at: previewNow(), rowUpdated: "9"})
	f.m.previewContent(60, 10)
	d := newLaunchDialog(tmuxInput())
	f.m.launchDlg = &d
	lw, _ := f.m.paneWidths()
	m := send(f.m, wheelMsg(tea.MouseButtonWheelDown, lw+5))
	if m.preview.top != 0 || m.cursor != 1 || m.launchDlg == nil {
		t.Errorf("the wheel acted behind the dialog: pane top=%d cursor=%d dialog=%v", m.preview.top, m.cursor, m.launchDlg != nil)
	}
}

func TestStaleAnswersAreDropped(t *testing.T) {
	installLaunch(t, map[string]string{"ORCA_WORKTREE_ID": "x"})
	m := issueOnScreen(t)
	next, _ := m.Update(k("a"))
	m = next.(model)
	if m.launchDlg == nil {
		t.Fatal("no dialog")
	}
	seq := m.launchSeq

	// models for an agent the dialog no longer shows
	m = pressLaunch(m, "e", "tab", "right") // devin
	next, _ = m.Update(launchModelsMsg{seq: seq, agent: "claude", models: []launch.Model{{}, {ID: "opus"}}})
	m = next.(model)
	for _, mm := range m.launchDlg.models {
		if mm.ID == "opus" {
			t.Error("claude's models were put in devin's dialog")
		}
	}

	// a base for a dialog that was closed and opened again
	m = pressLaunch(m, "esc")
	next, _ = m.Update(k("a"))
	m = next.(model)
	next, _ = m.Update(launchBaseMsg{seq: seq, base: "origin/old"})
	m = next.(model)
	if m.launchDlg.in.Base == "origin/old" {
		t.Error("a base for a closed dialog was used")
	}
	next, _ = m.Update(launchBaseMsg{seq: m.launchSeq, base: "origin/new"})
	if got := next.(model).launchDlg.in.Base; got != "origin/new" {
		t.Errorf("the current dialog's base = %q; want origin/new", got)
	}
	m = pressLaunch(m, "esc")
	next, _ = m.Update(launchBaseMsg{seq: m.launchSeq, base: "origin/x"})
	if next.(model).launchDlg != nil {
		t.Error("a base reopened a closed dialog")
	}

	// an issue for a row the cursor has left
	installIssues(t, issue42())
	l := issuesLoaded(t)
	l.settings, l.launchPrefs, l.curRepo = m.settings, m.launchPrefs, acme
	next, _ = l.Update(launchIssueMsg{id: l.itemID("999"), d: issue42()})
	if next.(model).launchDlg != nil {
		t.Error("an issue for another row opened the dialog")
	}
	next, _ = l.Update(launchIssueMsg{id: l.itemID("14812"), d: issue42()})
	if next.(model).launchDlg == nil {
		t.Error("the issue for the row on screen opened no dialog")
	}

	// a result for a dialog that is gone only updates the status on success
	m.launchDlg = nil
	next, _ = m.Update(launchDoneMsg{seq: 999, res: launch.Result{Where: "tmux pane"}})
	if nm := next.(model); nm.launchDlg != nil || !strings.Contains(ansi.Strip(nm.status), "launched: tmux pane") {
		t.Errorf("late success: dlg=%v status=%q", nm.launchDlg != nil, ansi.Strip(nm.status))
	}
	next, _ = m.Update(launchDoneMsg{seq: 999, err: errors.New("boom")})
	if nm := next.(model); nm.launchDlg != nil {
		t.Error("a late failure reopened the dialog")
	}
}

func TestLaunchOversizeIsShownInDialog(t *testing.T) {
	f := installLaunch(t, map[string]string{"ORCA_WORKTREE_ID": "x"})
	f.errs = []error{&launch.StepError{Step: launch.StepCreate, Err: errors.New("issue text is 600000 bytes; this platform can pass at most 524288 bytes to orca; trim the issue and try again")}}
	d := issue42()
	d.Body = strings.Repeat("長", launch.WarnChars+1)
	m := issueOnScreen(t)
	m = m.withScreenData(viewIssue, issueState{d: d})
	m = pressLaunch(m, "a")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "⚠ large issue") {
		t.Errorf("no size warning for a large issue:\n%s", v)
	}
	m = pressLaunch(m, "enter")
	if m.launchDlg == nil {
		t.Fatal("the dialog closed on an oversize refusal")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "failed at create workspace: issue text is 600000 bytes") {
		t.Errorf("oversize refusal not shown:\n%s", v)
	}
}

func TestPrefsSaveFailureIsOnlyAStatus(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	f.results = []launch.Result{{Where: "tmux pane"}}
	m := issueOnScreen(t)
	bad := filepath.Join(t.TempDir(), "launch.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.launchPrefs, _ = launch.LoadPrefs(bad)
	m = pressLaunch(m, "a", "e", "tab", "tab", "right", "enter") // pick a model so there is something to save
	if m.launchDlg != nil {
		t.Fatal("a save failure kept the dialog open")
	}
	st := ansi.Strip(m.status)
	if !strings.Contains(st, "launched: tmux pane") || !strings.Contains(st, "not remembered") {
		t.Errorf("status = %q; want the launch and a note", st)
	}
	if b, _ := os.ReadFile(bad); string(b) != "{not json" {
		t.Error("the unreadable prefs file was overwritten")
	}

	// a nil Prefs works too
	m = issueOnScreen(t)
	m.launchPrefs = nil
	if m = pressLaunch(m, "a", "enter"); !strings.Contains(ansi.Strip(m.status), "launched") {
		t.Errorf("nil prefs: status %q", ansi.Strip(m.status))
	}
}

func TestLaunchPrefsLoadProblemGoesToTheBanner(t *testing.T) {
	dir := t.TempDir()
	old := launchPrefsPath
	t.Cleanup(func() { launchPrefsPath = old })
	launchPrefsPath = func() string { return filepath.Join(dir, "launch.json") }
	if p, banner := loadLaunchPrefs(); p == nil || banner != "" {
		t.Errorf("missing file: prefs=%v banner=%q", p, banner)
	}
	if err := os.WriteFile(filepath.Join(dir, "launch.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, banner := loadLaunchPrefs(); p == nil || !strings.Contains(banner, "launch preferences") {
		t.Errorf("broken file: prefs=%v banner=%q", p, banner)
	}
}
