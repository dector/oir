package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// metaFile is the manifest written into each version directory.
const metaFile = ".oir.json"

// Meta records the artifact a version was installed from, so a later install
// can tell when a release republishes different bytes under the same version.
type Meta struct {
	Asset   string `json:"asset,omitempty"`
	Digest  string `json:"digest,omitempty"`
	AssetID int64  `json:"asset_id,omitempty"`
}

// ReadMeta returns the metadata recorded for a version. ok is false when no
// metadata has been written, for example installs made before metadata was
// tracked.
func (s *Store) ReadMeta(key, version string) (Meta, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir(key, version), metaFile))
	if errors.Is(err, os.ErrNotExist) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, err
	}

	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, false, err
	}

	return m, true, nil
}

// WriteMeta records metadata for a version, replacing any previous value.
func (s *Store) WriteMeta(key, version string, m Meta) error {
	dir := s.Dir(key, version)

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".oir-meta-*")
	if err != nil {
		return err
	}
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, metaFile)); err != nil {
		return err
	}
	tmp = nil

	return nil
}
