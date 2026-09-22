package gh

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceBinaryRaw(t *testing.T) {
	download := filepath.Join(t.TempDir(), "tool-1.2.3-linux-amd64")
	if err := os.WriteFile(download, []byte("\x7fELF raw binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	got, err := placeArtifact(download, "tool", dest, false)
	if err != nil {
		t.Fatalf("placeArtifact: %v", err)
	}
	if got != filepath.Join(dest, "tool") {
		t.Fatalf("got %q, want %q", got, filepath.Join(dest, "tool"))
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "\x7fELF raw binary" {
		t.Fatalf("binary content = %q", data)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("binary is not executable: %v", info.Mode())
	}
}

func TestPlaceBinaryArchive(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho hi\n")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "bin/tool",
		Mode:     0o755,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	download := filepath.Join(t.TempDir(), "tool-1.2.3-linux-amd64.tar.gz")
	if err := os.WriteFile(download, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	got, err := placeArtifact(download, "tool", dest, false)
	if err != nil {
		t.Fatalf("placeArtifact: %v", err)
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(body) {
		t.Fatalf("binary content = %q, want %q", data, body)
	}
}

func TestPlaceArtifactFull(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	entries := []struct {
		name string
		body string
		mode int64
	}{
		{"pi/pi", "#!/bin/sh\necho pi\n", 0o755},
		{"pi/theme/dark.json", `{"name":"dark"}`, 0o644},
		{"pi/package.json", `{"version":"1.0.0"}`, 0o644},
	}
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	download := filepath.Join(t.TempDir(), "pi-linux-x64.tar.gz")
	if err := os.WriteFile(download, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	got, err := placeArtifact(download, "pi", dest, true)
	if err != nil {
		t.Fatalf("placeArtifact: %v", err)
	}
	if got != filepath.Join(dest, "pi") {
		t.Fatalf("binary = %q, want %q", got, filepath.Join(dest, "pi"))
	}

	// The single top-level directory is collapsed and every sibling file is kept.
	for _, rel := range []string{"theme/dark.json", "package.json"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("%s not kept: %v", rel, err)
		}
	}
}
