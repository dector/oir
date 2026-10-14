package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCacheLifecycle(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	manifest := "version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\"\n"
	data := "tool = gh:o/tool\n"
	fail := false
	requests := 0
	warnings := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if fail {
			http.Error(w, "offline", 503)
			return
		}
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte(manifest))
		} else {
			w.Write([]byte(data))
		}
	}))
	defer server.Close()
	c, err := New(Options{Sources: []Source{{"test", server.URL + "/index.kdl"}}, CacheDir: t.TempDir(), Now: func() time.Time { return now }, Warn: func(error) { warnings++ }})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if e, err := c.Entries(ctx); err != nil || len(e) != 1 {
		t.Fatalf("first: %v %v", e, err)
	}
	path := c.path(c.options.Sources[0])
	original, _ := os.ReadFile(path)
	if _, err := c.Entries(ctx); err != nil || requests != 2 {
		t.Fatalf("fresh: %v requests %d", err, requests)
	}
	now = now.Add(25 * time.Hour)
	fail = true
	if _, err := c.Entries(ctx); err != nil || warnings != 1 {
		t.Fatalf("fallback: %v warnings %d", err, warnings)
	}
	if err := c.Refresh(ctx); err == nil {
		t.Fatal("forced refresh must fail")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("failed refresh changed snapshot")
	}
	fail = false
	for _, bad := range []string{"garbage", "version \"2\"\naliases file=\"aliases.oini\" type=\"oir-aliases\"", "version \"1\"\naliases file=\"../aliases.oini\" type=\"oir-aliases\"", "version \"1\"\naliases file=\"aliases.oini\" type=\"new\""} {
		old := manifest
		manifest = bad
		if err := c.Refresh(ctx); err == nil {
			t.Fatalf("accepted %q", bad)
		}
		manifest = old
		after, _ = os.ReadFile(path)
		if string(after) != string(original) {
			t.Fatal("invalid manifest changed snapshot")
		}
	}
	for _, invalid := range []string{"bad = nonsense\n", "tool = gh:o/tool@v1 bad\n", "tool = gh:o/tool@v1\x00bad\n"} {
		data = invalid
		if err := c.Refresh(ctx); err == nil {
			t.Fatal("accepted bad alias")
		}
		after, _ = os.ReadFile(path)
		if string(after) != string(original) {
			t.Fatal("invalid aliases changed snapshot")
		}
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	fail = true
	if _, err := c.Entries(ctx); err == nil {
		t.Fatal("corrupt cache used")
	}
	fail = false
	data = "new = gh:o/new\n"
	if e, err := c.Entries(ctx); err != nil || e[0].Key != "new" {
		t.Fatalf("repair: %v %v", e, err)
	}
}
func TestRefreshAllSources(t *testing.T) {
	hits := map[string]int{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if strings.HasPrefix(r.URL.Path, "/bad") {
			http.Error(w, "no", 500)
			return
		}
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte("version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\""))
		} else {
			w.Write([]byte("tool = gh:o/tool"))
		}
	}))
	defer s.Close()
	c, _ := New(Options{CacheDir: t.TempDir(), Sources: []Source{{"bad", s.URL + "/bad/index.kdl"}, {"good", s.URL + "/good/index.kdl"}, {"bad2", s.URL + "/bad2/index.kdl"}}})
	err := c.Refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "2 registries") || !strings.Contains(err.Error(), "bad2") {
		t.Fatalf("aggregate: %v", err)
	}
	if hits["/good/aliases.oini"] != 1 {
		t.Fatal("did not continue")
	}
	if _, _, err := c.load(c.options.Sources[1]); err != nil {
		t.Fatal(err)
	}
}
func TestCacheDirectory(t *testing.T) {
	t.Setenv("OIR_CACHE_DIR", "/oir-cache")
	t.Setenv("XDG_CACHE_HOME", "/xdg-cache")
	if got, _ := DefaultCacheDir(); got != "/oir-cache" {
		t.Fatal(got)
	}
	t.Setenv("OIR_CACHE_DIR", "")
	if got, _ := DefaultCacheDir(); got != "/xdg-cache/oir" {
		t.Fatal(got)
	}
}
