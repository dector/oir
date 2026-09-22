package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dector/oir/internal/gh"
	"github.com/dector/oir/internal/registry"
	"github.com/dector/oir/internal/spec"
)

const assetName = "tool-linux-amd64.tar.gz"

// makeArchive builds a .tar.gz containing a single executable named "tool".
func makeArchive(t *testing.T, content string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name:     "tool",
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// makePackageArchive builds a .tar.gz that wraps a binary and sibling runtime
// files in a single top-level directory, like most release tarballs.
func makePackageArchive(t *testing.T, content, theme string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	entries := []struct {
		name string
		body string
		mode int64
	}{
		{"tool/tool", content, 0o755},
		{"tool/theme/dark.json", theme, 0o644},
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

	return buf.Bytes()
}

func sha256Of(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// fakeGitHub serves a release plus its asset.
type fakeGitHub struct {
	downloads atomic.Int32
	archive   []byte
	digest    string // digest reported in the API, empty to omit
	checksums bool   // publish a checksums.txt asset instead of a digest
	corrupt   bool   // serve bytes that do not match the digest
}

func (f *fakeGitHub) writeRelease(w http.ResponseWriter, r *http.Request, tag string) {
	base := "http://" + r.Host
	asset := map[string]any{
		"id":                   1,
		"name":                 assetName,
		"size":                 len(f.archive),
		"browser_download_url": base + "/assets/" + assetName,
		"created_at":           "2026-01-01T00:00:00Z",
	}
	if f.digest != "" {
		asset["digest"] = f.digest
	}

	assets := []any{asset}
	if f.checksums {
		assets = append(assets, map[string]any{
			"id":                   2,
			"name":                 "checksums.txt",
			"size":                 len(f.archive),
			"browser_download_url": base + "/assets/checksums.txt",
			"created_at":           "2026-01-01T00:00:00Z",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":           99,
		"tag_name":     tag,
		"assets":       assets,
		"published_at": "2026-01-01T00:00:00Z",
	})
}

func (f *fakeGitHub) start(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/tool/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.writeRelease(w, r, "latest")
	})
	mux.HandleFunc("/repos/o/tool/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		f.writeRelease(w, r, path.Base(r.URL.Path))
	})

	mux.HandleFunc("/assets/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		f.downloads.Add(1)
		if f.corrupt {
			_, _ = w.Write([]byte("tampered"))
			return
		}
		_, _ = w.Write(f.archive)
	})

	mux.HandleFunc("/assets/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sha256Of(f.archive), assetName)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func newInstaller(t *testing.T, f *fakeGitHub, dataDir, binDir string) *Installer {
	t.Helper()

	srv := f.start(t)

	return New(Options{
		Backends: registry.Backends{
			string(spec.BackendGitHub): &gh.Client{BaseURL: srv.URL, HTTP: srv.Client(), UserAgent: "oir-test"},
		},
		StoreDir: dataDir,
		BinDir:   binDir,
		Platform: registry.Platform{OS: "linux", Arch: "amd64"},
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
}

func TestRunInstallsAndLinks(t *testing.T) {
	archive := makeArchive(t, "#!/bin/sh\necho hello\n")
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	res, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Version != "latest" {
		t.Errorf("version = %q, want latest", res.Version)
	}

	body, err := os.ReadFile(res.Binary)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(body) != "#!/bin/sh\necho hello\n" {
		t.Errorf("binary content = %q", body)
	}

	link, err := os.Readlink(filepath.Join(binDir, "tool"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if link != res.Binary {
		t.Errorf("link = %q, want %q", link, res.Binary)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	first, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.UpToDate {
		t.Error("first Run UpToDate = true, want false")
	}
	second, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !second.UpToDate {
		t.Error("second Run UpToDate = false, want true")
	}

	if got := f.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (second run should skip)", got)
	}
}

func TestRunReportsSkippedInstall(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	var stdout bytes.Buffer
	in.Stdout = &stdout

	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "skipping") {
		t.Errorf("second run output should mention skipping, got %q", got)
	}
	// "installed" may only appear as part of "already installed".
	if n := strings.Count(got, "installed"); n != 1 {
		t.Errorf("second run output mentions install %d times, want 1: %q", n, got)
	}
}

func TestRunVerifiesChecksumFromChecksumsFile(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive, checksums: true}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunRejectsTamperedDownload(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive), corrupt: true}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	if _, err := in.Run(context.Background(), sp); err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	if _, err := os.Lstat(filepath.Join(binDir, "tool")); !os.IsNotExist(err) {
		t.Error("a symlink was created for a failed install")
	}
}

func TestRunRequiresChecksumByDefault(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive} // no digest, no checksums file

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	if _, err := in.Run(context.Background(), sp); err == nil {
		t.Fatal("expected an error when no checksum is published")
	}

	in.NoVerify = true
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("Run with NoVerify: %v", err)
	}
}

func TestRunWarnsAndReinstallsWhenAssetChanges(t *testing.T) {
	dataDir, binDir := t.TempDir(), t.TempDir()
	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}

	first := makeArchive(t, "v1")
	f1 := &fakeGitHub{archive: first, digest: "sha256:" + sha256Of(first)}
	in1 := newInstaller(t, f1, dataDir, binDir)
	var stderr bytes.Buffer
	in1.Stderr = &stderr

	if _, err := in1.Run(context.Background(), sp); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	stderr.Reset()

	// Republish the same version with different bytes.
	second := makeArchive(t, "v2")
	f2 := &fakeGitHub{archive: second, digest: "sha256:" + sha256Of(second)}
	in2 := newInstaller(t, f2, dataDir, binDir)
	in2.Stderr = &stderr

	res, err := in2.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := f2.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (changed asset should reinstall)", got)
	}
	if !strings.Contains(stderr.String(), "warn:") {
		t.Errorf("stderr = %q, want a warn: line", stderr.String())
	}

	body, err := os.ReadFile(res.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v2" {
		t.Errorf("installed binary = %q, want v2", body)
	}
}

func TestRunUsesRequestedVersion(t *testing.T) {
	archive := makeArchive(t, "binary")
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool", Version: "v1.2.3"}
	res, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", res.Version)
	}
	want := filepath.Join(dataDir, "installs", "github", "o", "tool", "v1.2.3", "tool")
	if res.Binary != want {
		t.Errorf("binary = %q, want %q", res.Binary, want)
	}
}

func TestRunFullKeepsPackageAndIsSticky(t *testing.T) {
	archive := makePackageArchive(t, "#!/bin/sh\necho hello\n", `{"theme":"dark"}`)
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, f, dataDir, binDir)
	in.Full = true

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	res, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	versionDir := filepath.Join(dataDir, "installs", "github", "o", "tool", "latest")
	if res.Binary != filepath.Join(versionDir, "tool") {
		t.Errorf("binary = %q, want %q", res.Binary, filepath.Join(versionDir, "tool"))
	}
	theme := filepath.Join(versionDir, "theme", "dark.json")
	if _, err := os.Stat(theme); err != nil {
		t.Fatalf("theme not kept: %v", err)
	}

	meta, ok, err := in.Store.ReadMeta("github/o/tool", "latest")
	if err != nil || !ok {
		t.Fatalf("ReadMeta = (%v, %v), want ok", ok, err)
	}
	if !meta.Full || meta.Binary != "tool" {
		t.Errorf("meta = %+v, want Full with Binary tool", meta)
	}

	// A later run without --full keeps the package layout: --full is sticky.
	in.Full = false
	res2, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !res2.UpToDate {
		t.Error("second Run UpToDate = false, want true")
	}
	if got := f.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (sticky --full must not reinstall)", got)
	}
	if _, err := os.Stat(theme); err != nil {
		t.Errorf("theme removed by the second run: %v", err)
	}
}

func TestRunFullUpgradesSingleInstall(t *testing.T) {
	archive := makePackageArchive(t, "binary", `{"theme":"dark"}`)
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}

	in1 := newInstaller(t, f, dataDir, binDir)
	if _, err := in1.Run(context.Background(), sp); err != nil {
		t.Fatalf("single Run: %v", err)
	}

	versionDir := filepath.Join(dataDir, "installs", "github", "o", "tool", "latest")
	theme := filepath.Join(versionDir, "theme", "dark.json")
	if _, err := os.Stat(theme); err == nil {
		t.Fatal("theme present after a single-binary install")
	}

	in2 := newInstaller(t, f, dataDir, binDir)
	in2.Full = true
	res, err := in2.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("full Run: %v", err)
	}
	if res.UpToDate {
		t.Error("full Run UpToDate = true, want a reinstall to the package layout")
	}
	if _, err := os.Stat(theme); err != nil {
		t.Errorf("theme not kept after --full: %v", err)
	}
}

func TestRunOnlyBinaryKeepsThenClearsPackage(t *testing.T) {
	archive := makePackageArchive(t, "binary", `{"theme":"dark"}`)
	f := &fakeGitHub{archive: archive, digest: "sha256:" + sha256Of(archive)}

	dataDir, binDir := t.TempDir(), t.TempDir()
	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}

	in := newInstaller(t, f, dataDir, binDir)
	in.Full = true
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("full Run: %v", err)
	}

	versionDir := filepath.Join(dataDir, "installs", "github", "o", "tool", "latest")
	theme := filepath.Join(versionDir, "theme", "dark.json")

	// --only-binary flips the mode without touching files or downloading.
	in.Full = false
	in.OnlyBinary = true
	res, err := in.Run(context.Background(), sp)
	if err != nil {
		t.Fatalf("--only-binary Run: %v", err)
	}
	if !res.UpToDate {
		t.Error("--only-binary Run UpToDate = false, want true")
	}
	if _, err := os.Stat(theme); err != nil {
		t.Errorf("package files removed without --clear: %v", err)
	}
	if got := f.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (mode switch must not reinstall)", got)
	}
	meta, _, _ := in.Store.ReadMeta("github/o/tool", "latest")
	if meta.Full || meta.Binary != "tool" {
		t.Errorf("meta = %+v, want binary mode with Binary tool", meta)
	}

	// A plain run keeps binary mode and leaves the package files alone.
	in.OnlyBinary = false
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("plain Run: %v", err)
	}
	if _, err := os.Stat(theme); err != nil {
		t.Errorf("package files removed by a plain run: %v", err)
	}

	// --only-binary --clear removes the package files, keeping the binary.
	in.OnlyBinary = true
	in.Clear = true
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("--only-binary --clear Run: %v", err)
	}
	if _, err := os.Stat(theme); err == nil {
		t.Error("package files still present after --clear")
	}
	if _, err := os.Stat(filepath.Join(versionDir, "tool")); err != nil {
		t.Errorf("binary missing after --clear: %v", err)
	}
	if got := f.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (--clear must not reinstall)", got)
	}
}

func TestRunRejectsFullAndOnlyBinary(t *testing.T) {
	dataDir, binDir := t.TempDir(), t.TempDir()
	in := newInstaller(t, &fakeGitHub{}, dataDir, binDir)
	in.Full = true
	in.OnlyBinary = true

	sp := spec.Spec{Backend: spec.BackendGitHub, Owner: "o", Repo: "tool"}
	if _, err := in.Run(context.Background(), sp); err == nil {
		t.Fatal("expected an error for --full with --only-binary")
	}
}
