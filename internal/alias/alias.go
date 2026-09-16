// Package alias stores short names for tool specs.
//
// Aliases are user configuration, so they live under the config directory,
// not the data directory that holds installed tools. Each alias is a line
// of the form "key = gh:owner/repo" in the aliases file.
package alias

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dector/oir/internal/spec"
)

// File is an alias file at a fixed path.
type File struct {
	Path string
}

// Entry is a single alias.
type Entry struct {
	Key  string
	Spec string
}

var keyRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// DefaultPath returns the alias file path: $OIR_CONFIG_DIR/aliases,
// $XDG_CONFIG_HOME/oir/aliases or ~/.config/oir/aliases.
func DefaultPath() (string, error) {
	if dir := os.Getenv("OIR_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "aliases"), nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "oir", "aliases"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config", "oir", "aliases"), nil
}

// Open returns a File for the given path. The file is created on first write.
func Open(path string) *File {
	return &File{Path: path}
}

// List returns all aliases sorted by key.
func (f *File) List() ([]Entry, error) {
	m, err := f.load()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]Entry, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, Entry{Key: k, Spec: m[k]})
	}

	return entries, nil
}

// Get returns the spec registered for key.
func (f *File) Get(key string) (string, bool, error) {
	m, err := f.load()
	if err != nil {
		return "", false, err
	}

	value, ok := m[key]

	return value, ok, nil
}

// Set registers key, replacing any existing alias.
func (f *File) Set(key, value string) error {
	if !keyRE.MatchString(key) {
		return fmt.Errorf("invalid alias name %q", key)
	}
	if _, err := spec.Parse(value); err != nil {
		return err
	}

	m, err := f.load()
	if err != nil {
		return err
	}
	m[key] = strings.TrimSpace(value)

	return f.save(m)
}

// Remove deletes an alias and reports whether it existed.
func (f *File) Remove(key string) (bool, error) {
	m, err := f.load()
	if err != nil {
		return false, err
	}
	if _, ok := m[key]; !ok {
		return false, nil
	}
	delete(m, key)

	return true, f.save(m)
}

// Resolve turns an install argument into a Spec.
//
// Anything containing "/" or ":" is parsed as a spec. A bare name is looked up
// as an alias, with an optional "@version" override.
func Resolve(raw string) (spec.Spec, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.ContainsAny(trimmed, "/:") {
		return spec.Parse(trimmed)
	}

	name, version, _ := strings.Cut(trimmed, "@")

	path, err := DefaultPath()
	if err != nil {
		return spec.Spec{}, err
	}

	value, ok, err := Open(path).Get(name)
	if err != nil {
		return spec.Spec{}, err
	}
	if !ok {
		return spec.Spec{}, fmt.Errorf("no tool registered with name %q", name)
	}

	sp, err := spec.Parse(value)
	if err != nil {
		return spec.Spec{}, fmt.Errorf("alias %q: %w", name, err)
	}
	if version != "" {
		sp.Version = version
	}

	return sp, nil
}

// load reads the file into a map. A missing file is an empty map.
func (f *File) load() (map[string]string, error) {
	m := map[string]string{}

	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}

	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("%s:%d: want \"key = spec\"", f.Path, i+1)
		}

		m[key] = value
	}

	return m, nil
}

// save writes the map back atomically, replacing the previous file.
func (f *File) save(m map[string]string) error {
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, m[k])
	}

	tmp, err := os.CreateTemp(dir, "aliases.tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.WriteString(b.String()); err != nil {
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.Path); err != nil {
		return err
	}
	tmp = nil

	return nil
}
