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
	"sync/atomic"
	"testing"

	"github.com/dector/oir/internal/gh"
	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
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

	return &Installer{
		Client:   &gh.Client{BaseURL: srv.URL, HTTP: srv.Client(), UserAgent: "oir-test"},
		Store:    &store.Store{Root: dataDir},
		BinDir:   binDir,
		Platform: gh.Platform{OS: "linux", Arch: "amd64"},
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
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
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if _, err := in.Run(context.Background(), sp); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := f.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (second run should skip)", got)
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
