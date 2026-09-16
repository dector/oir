package gh

import "testing"

func asset(name string) Asset {
	return Asset{Name: name}
}

func TestPickAsset(t *testing.T) {
	linuxAMD64 := Platform{OS: "linux", Arch: "amd64"}
	darwinARM64 := Platform{OS: "darwin", Arch: "arm64"}

	tests := []struct {
		name    string
		assets  []string
		repo    string
		p       Platform
		want    string
		wantErr bool
	}{
		{
			name:   "rolling release with commit suffix",
			assets: []string{"ror-linux-amd64-df32f6e.tar.gz"},
			repo:   "ror",
			p:      linuxAMD64,
			want:   "ror-linux-amd64-df32f6e.tar.gz",
		},
		{
			name: "picks the matching platform",
			assets: []string{
				"tool-darwin-arm64.tar.gz",
				"tool-linux-amd64.tar.gz",
				"tool-windows-amd64.zip",
			},
			repo: "tool",
			p:    linuxAMD64,
			want: "tool-linux-amd64.tar.gz",
		},
		{
			name: "prefers plain build over musl",
			assets: []string{
				"tool-linux-amd64-musl.tar.gz",
				"tool-linux-amd64.tar.gz",
			},
			repo: "tool",
			p:    linuxAMD64,
			want: "tool-linux-amd64.tar.gz",
		},
		{
			name:   "accepts musl when it is the only option",
			assets: []string{"tool-x86_64-unknown-linux-musl.tar.gz"},
			repo:   "tool",
			p:      linuxAMD64,
			want:   "tool-x86_64-unknown-linux-musl.tar.gz",
		},
		{
			name:   "x86_64 alias",
			assets: []string{"tool-v1.0.0-x86_64-unknown-linux-gnu.tar.gz"},
			repo:   "tool",
			p:      linuxAMD64,
			want:   "tool-v1.0.0-x86_64-unknown-linux-gnu.tar.gz",
		},
		{
			name:   "macos alias",
			assets: []string{"tool-macos-aarch64.tar.gz"},
			repo:   "tool",
			p:      darwinARM64,
			want:   "tool-macos-aarch64.tar.gz",
		},
		{
			name: "ignores checksum files",
			assets: []string{
				"tool-linux-amd64.tar.gz.sha256",
				"tool-linux-amd64-checksums.txt",
				"tool-linux-amd64.tar.gz",
			},
			repo: "tool",
			p:    linuxAMD64,
			want: "tool-linux-amd64.tar.gz",
		},
		{
			name:    "no matching platform",
			assets:  []string{"tool-darwin-arm64.tar.gz"},
			repo:    "tool",
			p:       linuxAMD64,
			wantErr: true,
		},
		{
			name:    "unsupported archive format",
			assets:  []string{"tool-linux-amd64.tar.xz"},
			repo:    "tool",
			p:       linuxAMD64,
			wantErr: true,
		},
		{
			name:    "empty asset list",
			assets:  nil,
			repo:    "tool",
			p:       linuxAMD64,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := make([]Asset, 0, len(tt.assets))
			for _, n := range tt.assets {
				assets = append(assets, asset(n))
			}

			got, err := PickAsset(assets, tt.repo, tt.p)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got.Name)
				}
				return
			}
			if err != nil {
				t.Fatalf("PickAsset: %v", err)
			}
			if got.Name != tt.want {
				t.Fatalf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ror-linux-amd64-df32f6e.tar.gz", "ror linux amd64 df32f6e tar gz"},
		{"tool_x86_64_linux", "tool x86 64 linux"},
		{"Tool-MacOS-arm64", "tool macos arm64"},
	}
	for _, tt := range tests {
		if got := normalize(tt.in); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
