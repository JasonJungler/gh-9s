package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Prefs remembers the model the user last typed for each agent, in a small personal JSON file. A nil
// *Prefs is safe to use: it remembers nothing and saves nothing.
type Prefs struct {
	path     string
	models   map[string]string // agent id -> model; absent means the agent's default
	readOnly error             // why the file could not be read; while set, Save never writes
}

// prefsFile is the file's shape.
type prefsFile struct {
	Models map[string]string `json:"models"`
}

// LoadPrefs reads the remembered models at path. It never returns nil prefs: a missing or empty file is
// empty prefs; a file that cannot be read or understood gives empty prefs and an error saying why, and
// those prefs never overwrite the file.
func LoadPrefs(path string) (*Prefs, error) {
	p := &Prefs{path: path, models: map[string]string{}}
	if path == "" {
		p.readOnly = errors.New("there is no place to keep the launch preferences")
		return p, p.readOnly
	}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return p, nil
	case err != nil:
		p.readOnly = fmt.Errorf("launch preferences: %w", err)
		return p, p.readOnly
	case strings.TrimSpace(string(b)) == "":
		return p, nil
	}
	var f prefsFile
	if err := json.Unmarshal(b, &f); err != nil {
		p.readOnly = fmt.Errorf("launch preferences %s are not valid (%v); using defaults", path, err)
		return p, p.readOnly
	}
	for agent, model := range f.Models {
		if m := strings.TrimSpace(model); m != "" {
			p.models[agent] = m
		}
	}
	return p, nil
}

// Model is the model remembered for agentID, or "" for the agent's default.
func (p *Prefs) Model(agentID string) string {
	if p == nil {
		return ""
	}
	return p.models[agentID]
}

// SetModel remembers model for agentID; an empty model means the default and forgets the entry.
func (p *Prefs) SetModel(agentID, model string) {
	if p == nil {
		return
	}
	if model = strings.TrimSpace(model); model == "" {
		delete(p.models, agentID)
		return
	}
	p.models[agentID] = model
}

// Save writes the prefs atomically (temp file, then rename). Prefs whose file could not be read return
// that error and write nothing, so a file the user may be mid-edit on is never replaced.
func (p *Prefs) Save() error {
	if p == nil {
		return nil
	}
	if p.readOnly != nil {
		return p.readOnly
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(prefsFile{Models: p.models}, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}
