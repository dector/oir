package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	helloBody   = "hello world\n"
	helloSHA256 = "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestCheckSHA256(t *testing.T) {
	p := writeTemp(t, helloBody)

	for _, digest := range []string{helloSHA256, "sha256:" + helloSHA256, "SHA256:" + strings.ToUpper(helloSHA256)} {
		if err := CheckSHA256(p, digest); err != nil {
			t.Errorf("CheckSHA256(%q): %v", digest, err)
		}
	}

	if err := CheckSHA256(p, strings.Repeat("0", 64)); err == nil {
		t.Error("expected mismatch error")
	}
	if err := CheckSHA256(p, ""); err == nil {
		t.Error("expected error for empty digest")
	}
}

func TestFindChecksum(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		file  string
		want  string
		found bool
	}{
		{
			name:  "sha256sum two spaces",
			body:  helloSHA256 + "  tool-linux-amd64.tar.gz\n",
			file:  "tool-linux-amd64.tar.gz",
			want:  helloSHA256,
			found: true,
		},
		{
			name:  "binary mode star",
			body:  helloSHA256 + " *tool-linux-amd64.tar.gz\n",
			file:  "tool-linux-amd64.tar.gz",
			want:  helloSHA256,
			found: true,
		},
		{
			name:  "bsd style",
			body:  "SHA256 (tool-linux-amd64.tar.gz) = " + helloSHA256 + "\n",
			file:  "tool-linux-amd64.tar.gz",
			want:  helloSHA256,
			found: true,
		},
		{
			name:  "finds the right entry among many",
			body:  strings.Repeat("0", 64) + "  other.tar.gz\n" + helloSHA256 + "  tool.tar.gz\n",
			file:  "tool.tar.gz",
			want:  helloSHA256,
			found: true,
		},
		{
			name:  "skips comments and blanks",
			body:  "# comment\n\n" + helloSHA256 + "  tool.tar.gz\n",
			file:  "tool.tar.gz",
			want:  helloSHA256,
			found: true,
		},
		{
			name:  "missing entry",
			body:  helloSHA256 + "  other.tar.gz\n",
			file:  "tool.tar.gz",
			found: false,
		},
		{
			name:  "rejects non-hex digest",
			body:  "not-a-digest  tool.tar.gz\n",
			file:  "tool.tar.gz",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindChecksum(strings.NewReader(tt.body), tt.file)
			if !tt.found {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindChecksum: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
