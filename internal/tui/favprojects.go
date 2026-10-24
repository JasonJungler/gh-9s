package tui

import (
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/pins"
)

func init() {
	dialogHandlers["pin.project"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		if !r.ok {
			return m, nil
		}
		host, owner, n, err := pins.ParseProject(strings.TrimSpace(r.text))
		if err != nil {
			m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
			return m, nil
		}
		if m.pinStore().HasProject(host, owner, n) {
			m.status = styDim.Render(owner + "/#" + strconv.Itoa(n) + " is already pinned")
			return m, nil
		}
		m.status = styDim.Render("reading the Project…")
		return m, func() tea.Msg {
			p, err := fetchProject(host, owner, n)
			return projectPinMsg{host: host, owner: owner, number: n, p: p, err: err}
		}
	}
	dialogHandlers["pin.unproject"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pinDelProject
		m.pinDelProject = nil
		if !r.ok || p == nil {
			return m, nil
		}
		return m.togglePinnedProject(*p)
	}
}

// projectsTable lists the pinned Projects.
func projectsTable(st *pins.Store) table {
	t := table{cols: []string{"PROJECT", "WHERE"}}
	for _, p := range st.Projects {
		t.rows = append(t.rows, row{id: projectKey(p), extra: projectKey(p), url: p.URL, cells: []string{ghcli.CleanText(p.Title), projectKey(p)}})
	}
	return t
}

// projectKey is [HOST/]OWNER/#N, which also identifies the Project on screen.
func projectKey(p pins.Project) string {
	s := p.Owner + "/#" + strconv.Itoa(p.Number)
	if p.Host != "" {
		s = p.Host + "/" + s
	}
	return s
}

func (m model) selectedProject() (pins.Project, bool) {
	r, ok := m.selected()
	if !ok {
		return pins.Project{}, false
	}
	for _, p := range m.pinStore().Projects {
		if projectKey(p) == r.id {
			return p, true
		}
	}
	return pins.Project{}, false
}

func (m model) projectTabKey(k tea.KeyMsg) (model, tea.Cmd, bool) {
	switch k.String() {
	case "enter":
		p, ok := m.selectedProject()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.openBoard(p)
		return next, cmd, true
	case "a":
		m.dlg = &dialog{id: "pin.project", title: "Pin a Project", prompt: "[HOST/]OWNER/NUMBER", kind: dialogText}
		return m, nil, true
	case "f", "d":
		p, ok := m.selectedProject()
		if !ok {
			return m, nil, true
		}
		m.pinDelProject = &p
		d := newConfirmDialog("pin.unproject", "Unpin "+ghcli.CleanText(p.Title)+" ("+projectKey(p)+")?", "The Project itself is not touched; only the pin goes.")
		m.dlg = &d
		return m, nil, true
	case "o":
		p, ok := m.selectedProject()
		switch {
		case !ok:
		case p.URL == "":
			m.status = styDim.Render("no address stored for this Project; unpin and pin it again")
		default:
			m = m.open(p.URL)
			if m.status == "" {
				m.status = styDim.Render("opened " + p.URL)
			}
		}
		return m, nil, true
	case "t", "e":
		m.status = styDim.Render("pinned Projects: enter opens the board")
		return m, nil, true
	}
	return m, nil, false
}

// projectPinMsg is the outcome of reading a Project that is about to be pinned.
type projectPinMsg struct {
	host, owner string
	number      int
	p           ghcli.Project
	err         error
}

func (m model) onProjectPin(msg projectPinMsg) (model, tea.Cmd) {
	if msg.err != nil {
		text := ghcli.CleanText(msg.err.Error())
		if errors.Is(msg.err, ghcli.ErrMissingScope) {
			text = ghcli.ScopeHint(msg.host)
		}
		m.status = styErr.Render("✗ " + msg.owner + "/#" + strconv.Itoa(msg.number) + ": " + text)
		return m, nil
	}
	return m.togglePinnedProject(pins.Project{Host: msg.host, Owner: msg.owner, Number: msg.number, Title: msg.p.Title, URL: msg.p.URL})
}

func (m model) togglePinnedProject(p pins.Project) (model, tea.Cmd) {
	on, err := m.pinStore().ToggleProject(p)
	if err != nil {
		m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
		return m, nil
	}
	if m.pins == nil {
		m.pins = m.pinStore()
	}
	done := "removed pin " + projectKey(p)
	if on {
		done = "★ pinned " + ghcli.CleanText(p.Title) + " (" + projectKey(p) + ")"
	}
	m = m.savePins(done)
	if m.view == viewFavs {
		return m, m.fetch()
	}
	return m, nil
}
