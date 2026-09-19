// Package archive extracts release archives safely.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupportedFormat is returned for archives oir cannot extract.
var ErrUnsupportedFormat = errors.New("unsupported archive format")

// Extract unpacks the archive at src into destDir.
//
// It supports .tar.gz/.tgz and .zip, detected from the file content rather
// than the file name. Paths that escape destDir and symlinks are rejected.
func Extract(src, destDir string) error {
	kind, err := sniff(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	switch kind {
	case kindTarGz:
		return extractTarGz(src, destDir)
	case kindZip:
		return extractZip(src, destDir)
	default:
		return ErrUnsupportedFormat
	}
}

// IsArchive reports whether src looks like a supported archive, based on its
// content. Plain, uncompressed binaries return false with a nil error.
func IsArchive(src string) (bool, error) {
	kind, err := sniff(src)
	if errors.Is(err, ErrUnsupportedFormat) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return kind != kindUnknown, nil
}

type kind int

const (
	kindUnknown kind = iota
	kindTarGz
	kindZip
)

func sniff(src string) (kind, error) {
	f, err := os.Open(src)
	if err != nil {
		return kindUnknown, err
	}
	defer f.Close()

	var magic [4]byte
	n, err := io.ReadFull(f, magic[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return kindUnknown, err
	}
	if n < 4 {
		return kindUnknown, fmt.Errorf("%w: file too small", ErrUnsupportedFormat)
	}

	switch {
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return kindTarGz, nil
	case magic[0] == 'P' && magic[1] == 'K' && magic[2] == 0x03 && magic[3] == 0x04:
		return kindZip, nil
	default:
		return kindUnknown, ErrUnsupportedFormat
	}
}

func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		target, err := secureJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, os.FileMode(hdr.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			// Skipped on purpose: a link could point outside the extraction dir.
			continue
		default:
			continue
		}
	}
}

func extractZip(src, destDir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, zf := range zr.File {
		target, err := secureJoin(destDir, zf.Name)
		if err != nil {
			return err
		}

		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if zf.Mode()&os.ModeSymlink != 0 {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, rc, zf.Mode().Perm())
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func writeFile(target string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	if perm == 0 {
		perm = 0o644
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return err
	}

	return out.Close()
}

// secureJoin joins name onto dest, rejecting absolute paths and traversal.
func secureJoin(dest, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}

	target := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}

	return target, nil
}
