package catalog

import (
	"strings"
	"testing"
)

func TestRegistryAliasTagValidation(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "release/v1", "v1+build", "2026-01-01", "latest"} {
		if _, err := ParseAliases([]byte("tool = gh:o/tool@" + tag)); err != nil {
			t.Errorf("valid %q: %v", tag, err)
		}
	}
	for _, tag := range []string{"v1 bad", "v1\tbad", "v1\x00bad", "v1\x7fbad", "v1\u00a0bad", "v1~bad", "v1^bad", "v1:bad", "v1?bad", "v1*bad", "v1[bad", "v1\\bad", "v1..bad", "v1@{bad", "@", "/v1", "v1/", "v1//bad", ".v1", "release/.v1", "release/v1.lock", "v1."} {
		if _, err := ParseAliases([]byte("tool = gh:o/tool@" + tag)); err == nil || !strings.Contains(err.Error(), "invalid Git tag") {
			t.Errorf("invalid %q: %v", tag, err)
		}
	}
}
