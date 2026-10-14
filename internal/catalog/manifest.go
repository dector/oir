// Package catalog parses name-resolution registry data, independently of
// installation backends and user-owned alias storage.
package catalog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dector/kdly"
	"github.com/dector/oir/internal/alias"
)

// Manifest describes the sibling alias file published beside index.kdl.
type Manifest struct {
	Version string
	File    string
	Type    string
}

// ParseManifest validates a schema-version-1 index.kdl. It performs no I/O.
func ParseManifest(data []byte) (Manifest, error) {
	doc, err := kdly.Parse(string(data))
	if err != nil {
		return Manifest{}, fmt.Errorf("index.kdl: %w", err)
	}
	var m Manifest
	var versions, aliases int
	for _, node := range doc.Nodes {
		if node.TypeAnnotation != "" || len(node.Children) != 0 {
			return Manifest{}, fmt.Errorf("index.kdl: %q must be an untyped leaf node", node.Name)
		}
		switch node.Name {
		case "version":
			versions++
			if versions != 1 || len(node.Arguments) != 1 || len(node.Properties) != 0 || !plainString(node.Arguments[0]) {
				return Manifest{}, fmt.Errorf("index.kdl: want exactly one version node with one string argument")
			}
			m.Version = node.Arguments[0].Value
		case "aliases":
			aliases++
			if aliases != 1 || len(node.Arguments) != 0 {
				return Manifest{}, fmt.Errorf("index.kdl: want exactly one aliases entry with file and type properties")
			}
			for _, prop := range node.Properties {
				if !plainString(prop.Value) {
					return Manifest{}, fmt.Errorf("index.kdl: aliases %q must be a string", prop.Key)
				}
				switch prop.Key {
				case "file":
					m.File = prop.Value.Value
				case "type":
					m.Type = prop.Value.Value
				default:
					return Manifest{}, fmt.Errorf("index.kdl: unknown aliases property %q", prop.Key)
				}
			}
		default:
			return Manifest{}, fmt.Errorf("index.kdl: unknown node %q", node.Name)
		}
	}
	if versions != 1 {
		return Manifest{}, fmt.Errorf("index.kdl: missing version node")
	}
	if m.Version != "1" {
		return Manifest{}, fmt.Errorf("index.kdl: unsupported schema version %q; oir upgrade required", m.Version)
	}
	if aliases != 1 {
		return Manifest{}, fmt.Errorf("index.kdl: want exactly one aliases entry")
	}
	if m.File == "" || m.Type == "" {
		return Manifest{}, fmt.Errorf("index.kdl: aliases requires nonempty file and type properties")
	}
	if m.Type != "oir-aliases" {
		return Manifest{}, fmt.Errorf("index.kdl: unsupported alias-file type %q; oir upgrade required", m.Type)
	}
	if !safeFilename(m.File) {
		return Manifest{}, fmt.Errorf("index.kdl: unsafe alias filename %q: want a flat sibling filename", m.File)
	}
	return m, nil
}

func plainString(v kdly.Value) bool {
	return v.Type == kdly.ValueTypeString && v.TypeAnnotation == ""
}

// A portable, deliberately narrow filename alphabet excludes URL syntax,
// encoded traversal, separators, drive prefixes and control characters.
var filenameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func safeFilename(name string) bool {
	if !filenameRE.MatchString(name) || name == "." || strings.HasSuffix(name, ".") {
		return false
	}
	// Windows device names are reserved even when followed by an extension.
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}

// ParseAliases parses oini and validates each resulting registry entry.
// Sorting and duplicate handling are inherited from alias.Parse (last wins).
// Invalid data returns no partial entries.
func ParseAliases(data []byte) ([]alias.Entry, error) {
	entries, err := alias.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("registry aliases: %w", err)
	}
	for _, entry := range entries {
		if err := alias.Validate(entry.Key, entry.Spec); err != nil {
			return nil, fmt.Errorf("registry alias %q: %w", entry.Key, err)
		}
	}
	return entries, nil
}
