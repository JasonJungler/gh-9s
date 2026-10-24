package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Source says where a setting's current value came from.
type Source int

const (
	FromDefault Source = iota
	FromFile
	FromEnv
)

// ErrNotSaved wraps the reason a change was applied for this run but could not be written.
var ErrNotSaved = errors.New("not saved")

const maxFile = 10 << 20

// Store holds the settings of one run.
type Store struct {
	mu       sync.Mutex
	path     string
	file     map[string]any // values from the file (and carried over from the old prefs file)
	envVals  map[string]any // values given by the environment
	dirty    map[string]bool
	notes    []string
	readOnly error
}

// Load reads the settings. It never returns a nil store: if the file cannot be used the store holds
// the defaults, is read-only, and the error says why. getenv is os.Getenv (a seam); legacyPrefs is
// the path of the old prefs.json, whose delete-branch choice is carried over when the file lacks it.
func Load(path string, getenv func(string) string, legacyPrefs string) (*Store, error) {
	s := &Store{path: path, file: map[string]any{}, envVals: map[string]any{}, dirty: map[string]bool{}}
	var loadErr error
	if path == "" {
		loadErr = errors.New("there is no place to keep the settings file")
	} else {
		raw, err := readRaw(path)
		if err != nil {
			loadErr = err
		} else {
			for _, d := range Registry {
				rv, ok := raw[d.Key]
				if !ok {
					continue
				}
				if v, err := fromJSON(d, rv); err != nil {
					s.notes = append(s.notes, fmt.Sprintf("settings file: %s %s: using the default", d.Key, err))
				} else {
					s.file[d.Key] = v
				}
			}
			if _, has := raw["delete_branch_after_merge"]; !has && legacyPrefs != "" {
				if b, err := os.ReadFile(legacyPrefs); err == nil {
					var old struct {
						Delete *bool `json:"delete_branch_after_merge"`
					}
					if json.Unmarshal(b, &old) == nil && old.Delete != nil {
						s.file["delete_branch_after_merge"] = *old.Delete
						s.dirty["delete_branch_after_merge"] = true // written with the next save, then never read again
					}
				}
			}
		}
	}
	s.readOnly = loadErr
	for _, d := range Registry {
		if d.Env == "" || getenv == nil {
			continue
		}
		in := strings.TrimSpace(getenv(d.Env))
		if in == "" {
			continue
		}
		v, err := parse(d, in)
		if err == nil && d.Kind == Enum && len(d.EnvOptions) > 0 && !contains(d.EnvOptions, v.(string)) {
			err = fmt.Errorf("must be one of %s", strings.Join(d.EnvOptions, ", "))
		}
		if err != nil {
			s.notes = append(s.notes, fmt.Sprintf("%s=%s ignored: %s", d.Env, clip(in), err))
			continue
		}
		s.envVals[d.Key] = v
	}
	return s, loadErr
}

func contains(list []string, x string) bool {
	for _, y := range list {
		if y == x {
			return true
		}
	}
	return false
}

// clip makes a value fit on a note line: control characters out, short, quoted.
func clip(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len([]rune(out)) > 30 {
		out = string([]rune(out)[:30]) + "…"
	}
	return strconv.Quote(out)
}

func readRaw(path string) (map[string]json.RawMessage, error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, errors.New("the settings file is a directory")
	}
	if st.Size() > maxFile {
		return nil, errors.New("the settings file is too large")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("the settings file is not valid JSON (%v)", err)
	}
	return raw, nil
}

func fromJSON(d Def, rv json.RawMessage) (any, error) {
	if t := bytes.TrimSpace(rv); bytes.Equal(t, []byte("null")) {
		return nil, errors.New("must not be null")
	} else if d.Kind != Enum && len(t) > 0 && t[0] == '"' {
		return nil, errors.New("must not be in quotes")
	}
	switch d.Kind {
	case Bool:
		var v bool
		if json.Unmarshal(rv, &v) != nil {
			return nil, errors.New("must be true or false")
		}
		return v, nil
	case Int:
		var v int
		dec := json.NewDecoder(bytes.NewReader(rv))
		dec.UseNumber()
		var n json.Number
		if dec.Decode(&n) != nil {
			return nil, errors.New("must be a whole number")
		}
		x, err := strconv.Atoi(n.String())
		if err != nil {
			return nil, errors.New("must be a whole number")
		}
		v = x
		if v < d.Min || v > d.Max {
			return nil, fmt.Errorf("must be between %d and %d", d.Min, d.Max)
		}
		return v, nil
	}
	var v string
	if json.Unmarshal(rv, &v) != nil {
		return nil, errors.New("must be text")
	}
	return parse(d, v)
}

// parse turns what a person typed into a value of the setting's kind.
func parse(d Def, in string) (any, error) {
	in = strings.TrimSpace(in)
	switch d.Kind {
	case Bool:
		switch strings.ToLower(in) {
		case "on", "true", "yes", "1":
			return true, nil
		case "off", "false", "no", "0":
			return false, nil
		}
		return nil, errors.New("must be on or off")
	case Int:
		n, err := strconv.Atoi(in)
		if err != nil {
			return nil, errors.New("must be a whole number")
		}
		if n < d.Min || n > d.Max {
			return nil, fmt.Errorf("must be between %d and %d", d.Min, d.Max)
		}
		return n, nil
	}
	for _, o := range d.Options {
		if strings.EqualFold(in, o) {
			return o, nil
		}
	}
	return nil, fmt.Errorf("must be one of %s", strings.Join(d.Options, ", "))
}

func (s *Store) value(key string) (any, Source, bool) {
	d, ok := Lookup(key)
	if !ok {
		return nil, FromDefault, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.envVals[key]; ok {
		return v, FromEnv, true
	}
	if v, ok := s.file[key]; ok {
		return v, FromFile, true
	}
	return d.Default, FromDefault, true
}

// Bool, Int and String read a setting; an unknown key reads as the zero value.
func (s *Store) Bool(key string) bool { v, _, _ := s.value(key); b, _ := v.(bool); return b }
func (s *Store) Int(key string) int   { v, _, _ := s.value(key); n, _ := v.(int); return n }
func (s *Store) String(key string) string {
	v, _, _ := s.value(key)
	x, _ := v.(string)
	return x
}

// Source says where the setting's value comes from.
func (s *Store) Source(key string) Source { _, src, _ := s.value(key); return src }

// Overridden says whether the environment is overriding the setting.
func (s *Store) Overridden(key string) bool { return s.Source(key) == FromEnv }

// Notes are the problems found while loading, one line each.
func (s *Store) Notes() []string { return append([]string(nil), s.notes...) }

// ReadOnly is why changes cannot be saved, or nil.
func (s *Store) ReadOnly() error { return s.readOnly }

// Display is a value as it is shown and typed: on/off, the option, or the number.
func Display(d Def, v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "on"
		}
		return "off"
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	}
	return ""
}

// Set changes a setting for this run and saves it. The returned text is the normalised value. An
// invalid value changes nothing. If the change could not be saved it still applies, and the error
// wraps ErrNotSaved with the reason.
func (s *Store) Set(key, value string) (string, error) {
	d, ok := Lookup(key)
	if !ok {
		return "", fmt.Errorf("unknown setting %s", clip(key))
	}
	v, err := parse(d, value)
	if err != nil {
		return "", fmt.Errorf("%s %s", d.Key, err)
	}
	s.mu.Lock()
	s.file[d.Key] = v
	s.dirty[d.Key] = true
	s.mu.Unlock()
	if err := s.save(); err != nil {
		return Display(d, v), fmt.Errorf("%w: %v", ErrNotSaved, err)
	}
	return Display(d, v), nil
}

// Cycle moves a setting to its next (dir > 0) or previous value and saves it.
func (s *Store) Cycle(key string, dir int) (string, error) {
	d, ok := Lookup(key)
	if !ok {
		return "", fmt.Errorf("unknown setting %s", clip(key))
	}
	cur, _, _ := s.fileValue(d.Key)
	var next string
	switch d.Kind {
	case Bool:
		next = Display(d, !cur.(bool))
	case Int:
		step := 1
		if d.Max-d.Min > 30 { // a wide range moves in steps of 5
			step = 5
		}
		n := min(max(cur.(int)+dir*step, d.Min), d.Max)
		next = strconv.Itoa(n)
	default:
		i := 0
		for j, o := range d.Options {
			if o == cur.(string) {
				i = j
			}
		}
		next = d.Options[(i+dir+len(d.Options))%len(d.Options)]
	}
	return s.Set(d.Key, next)
}

// fileValue is the value as the file (or the default) has it, ignoring the environment: what a
// change starts from.
func (s *Store) fileValue(key string) (any, Source, bool) {
	d, _ := Lookup(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.file[key]; ok {
		return v, FromFile, true
	}
	return d.Default, FromDefault, true
}

// save writes the keys changed in this run into the file as it is now, so another gh-9s's changes
// and keys we do not know are kept.
func (s *Store) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly != nil {
		return s.readOnly
	}
	raw, err := readRaw(s.path)
	if err != nil {
		return err
	}
	for key := range s.dirty {
		b, err := json.Marshal(s.file[key])
		if err != nil {
			return err
		}
		raw[key] = b
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	target := s.path
	if real, err := filepath.EvalSymlinks(s.path); err == nil {
		target = real // a link to a dotfiles copy stays a link: the real file is the one rewritten
	}
	mode := os.FileMode(0o600)
	if st, err := os.Stat(target); err == nil {
		mode = st.Mode().Perm() // an existing file keeps its permissions
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), target); err != nil {
		return err
	}
	s.dirty = map[string]bool{} // written: later saves must not put these old values back over someone else's change
	return nil
}

var memory = func() *Store { s, _ := Load("", nil, ""); return s }()

// Memory is a store that keeps nothing on disk: defaults, with changes held for the run only. It is
// what code uses when no settings were loaded (tests, a failed start).
func Memory() *Store { return memory }

// Path is the settings file ("" when there is none).
func (s *Store) Path() string { return s.path }
