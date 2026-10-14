package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dector/oir/internal/catalog"
)

func TestRefreshCommand(t *testing.T) {
	useTestCatalog(t)
	if got := run(t, "refresh"); !strings.Contains(got, "registries refreshed") {
		t.Fatal(got)
	}
	if err := runErr(t, "refresh", "extra"); err == nil {
		t.Fatal("accepted argument")
	}
	if got := run(t, "help"); !strings.Contains(got, "refresh") {
		t.Fatal("help missing refresh")
	}
}
func TestRefreshReportsEveryFailureAndKeepsSuccess(t *testing.T) {
	hits := map[string]int{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if strings.HasPrefix(r.URL.Path, "/bad") {
			http.Error(w, "offline", 503)
			return
		}
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte("version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\""))
			return
		}
		w.Write([]byte("tool = gh:o/tool"))
	}))
	defer s.Close()
	sources := []catalog.Source{{Name: "bad-one", IndexURL: s.URL + "/bad1/index.kdl"}, {Name: "good", IndexURL: s.URL + "/good/index.kdl"}, {Name: "bad-two", IndexURL: s.URL + "/bad2/index.kdl"}}
	useCatalogSources(t, sources)
	var out bytes.Buffer
	root := New()
	root.Writer = &out
	root.ErrWriter = &out
	err := root.Run(context.Background(), []string{"oir", "refresh"})
	if err == nil {
		t.Fatal("expected failure")
	}
	for _, want := range []string{"bad-one", "bad-two", "2 registries"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %s: %s", want, &out)
		}
	}
	if hits["/good/aliases.oini"] != 1 {
		t.Fatal("success not attempted")
	}
	// A second explicit refresh must fetch even the fresh successful source.
	_ = root.Run(context.Background(), []string{"oir", "refresh"})
	if hits["/good/aliases.oini"] != 2 {
		t.Fatal("explicit refresh used a fresh cache")
	}
}
