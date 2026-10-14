package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dector/oir/internal/catalog"
)

func useTestCatalog(t *testing.T) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte("version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\""))
			return
		}
		w.Write([]byte("oir = gh:dector/oir\nror = gh:dector/ror\n"))
	}))
	t.Cleanup(s.Close)
	useCatalogSources(t, []catalog.Source{{Name: "official", IndexURL: s.URL + "/index.kdl"}})
}
func useCatalogSources(t *testing.T, sources []catalog.Source) {
	t.Helper()
	dir := t.TempDir()
	old := makeCatalog
	makeCatalog = func(o catalog.Options) (*catalog.Catalog, error) {
		o.Sources = sources
		o.CacheDir = dir
		return catalog.New(o)
	}
	t.Cleanup(func() { makeCatalog = old })
}
