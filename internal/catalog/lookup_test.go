package catalog

import (
	"github.com/dector/oir/internal/alias"
	"testing"
)

func TestMergePrecedence(t *testing.T) {
	got := Merge([]alias.Entry{{Key: "tool", Spec: "gh:user/tool"}}, []Entry{{"tool", "gh:first/tool", "first"}, {"other", "gh:first/other", "first"}, {"other", "gh:second/other", "second"}})
	if len(got) != 2 || got[0].Source != "first" || got[1].Source != "user" {
		t.Fatalf("winners: %v", got)
	}
}
