package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type entry struct {
	name string
	body string
	mode int64
}

func makeTarGz(t *testing.T, entries []entry) string {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

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

	p := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	return p
}

func makeZip(t *testing.T, entries []entry) string {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestExtractAndFindBinaryTarGz(t *testing.T) {
	src := makeTarGz(t, []entry{
		{name: "ror", body: "#!/bin/sh\n", mode: 0o755},
		{name: "README.md", body: "docs", mode: 0o644},
		{name: "completions/ror.bash", body: "comp", mode: 0o644},
	})

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got, err := FindBinary(dest, "ror")
	if err != nil {
		t.Fatalf("FindBinary: %v", err)
	}
	if filepath.Base(got) != "ror" {
		t.Fatalf("got %q, want the ror binary", got)
	}

	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Errorf("README.md not extracted: %v", err)
	}
}

func TestExtractZip(t *testing.T) {
	src := makeZip(t, []entry{
		{name: "tool.exe", body: "binary"},
		{name: "LICENSE", body: "license"},
	})

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got, err := FindBinary(dest, "tool")
	if err != nil {
		t.Fatalf("FindBinary: %v", err)
	}
	if filepath.Base(got) != "tool.exe" {
		t.Fatalf("got %q, want tool.exe", got)
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	src := makeTarGz(t, []entry{{name: "../evil.txt", body: "boom", mode: 0o644}})

	dest := t.TempDir()
	if err := Extract(src, dest); err == nil {
		t.Fatal("expected traversal to be rejected")
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Fatal("traversal file escaped the extraction dir")
	}
}

func TestExtractRejectsUnsupported(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(p, []byte("not an archive at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Extract(p, t.TempDir()); err == nil {
		t.Fatal("expected unsupported format error")
	}
}

func TestFindBinaryPrefersRepoName(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{"helper", "mytool", "other"} {
		if err := os.WriteFile(filepath.Join(dest, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := FindBinary(dest, "mytool")
	if err != nil {
		t.Fatalf("FindBinary: %v", err)
	}
	if filepath.Base(got) != "mytool" {
		t.Fatalf("got %q, want mytool", got)
	}
}

func TestFindBinaryNoCandidates(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := FindBinary(dest, "tool"); err == nil {
		t.Fatal("expected error when only docs are present")
	}
}
