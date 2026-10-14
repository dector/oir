package catalog

import (
	"context"
	"github.com/dector/oir/internal/alias"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	hits := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte("version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\""))
		} else {
			w.Write([]byte("tool = gh:registry/tool@old\n"))
		}
	}))
	defer s.Close()
	c, _ := New(Options{CacheDir: t.TempDir(), Sources: []Source{{"first", s.URL + "/index.kdl"}, {"second", s.URL + "/index.kdl"}}})
	ctx := context.Background()
	for _, tc := range []struct {
		raw            string
		users          []alias.Entry
		target, source string
		wantHits       int
	}{
		{"gh:direct/tool@v1", nil, "github:direct/tool@v1", "", 0},
		{"tool@v2", []alias.Entry{{Key: "tool", Spec: "gh:user/tool@old"}}, "github:user/tool@v2", "user", 0},
		{"tool@v3", nil, "github:registry/tool@v3", "first", 4},
		{"tool", nil, "github:registry/tool@old", "first", 4},
		{"tool@release/v1", nil, "github:registry/tool@release/v1", "first", 4},
	} {
		got, err := c.Resolve(ctx, tc.raw, tc.users)
		if err != nil || got.Spec.String() != tc.target || got.Source != tc.source || hits != tc.wantHits {
			t.Fatalf("%s: %+v %v hits=%d", tc.raw, got, err, hits)
		}
	}
	if _, err := c.Resolve(ctx, "missing", nil); err == nil || !strings.Contains(err.Error(), "oir refresh") {
		t.Fatalf("miss: %v", err)
	}
	if hits != 4 {
		t.Fatal("missing name caused refresh")
	}
}

func TestMergePrecedence(t *testing.T) {
	got := Merge([]alias.Entry{{Key: "tool", Spec: "gh:user/tool"}}, []Entry{{"tool", "gh:first/tool", "first"}, {"other", "gh:first/other", "first"}, {"other", "gh:second/other", "second"}})
	if len(got) != 2 || got[0].Source != "first" || got[1].Source != "user" {
		t.Fatalf("winners: %v", got)
	}
}
