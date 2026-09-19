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
	got, err := placeBinary(download, "tool", dest)
	if err != nil {
		t.Fatalf("placeBinary: %v", err)
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
	got, err := placeBinary(download, "tool", dest)
	if err != nil {
		t.Fatalf("placeBinary: %v", err)
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(body) {
		t.Fatalf("binary content = %q, want %q", data, body)
	}
}
