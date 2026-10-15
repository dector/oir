package registry

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}

	return t
}

// TestPickBunAndroidRegression guards against picking Bun's Android builds,
// which are named "bun-linux-<arch>-android.zip" and therefore also match a
// plain linux target. Capture times are those GitHub reported for v1.4.3.
func TestPickBunAndroidRegression(t *testing.T) {
	assets := []Asset{
		{Name: "bun-linux-x64-android-baseline-profile.zip", CreatedAt: at("2026-10-10T02:29:59Z")},
		{Name: "bun-linux-x64-android-baseline.zip", CreatedAt: at("2026-10-10T02:30:14Z")},
		{Name: "bun-linux-x64-android-profile.zip", CreatedAt: at("2026-10-10T02:30:17Z")},
		{Name: "bun-linux-x64-android.zip", CreatedAt: at("2026-10-10T02:30:35Z")},
		{Name: "bun-linux-x64-baseline-profile.zip", CreatedAt: at("2026-10-10T02:30:16Z")},
		{Name: "bun-linux-x64-baseline.zip", CreatedAt: at("2026-10-10T02:30:22Z")},
		{Name: "bun-linux-x64-musl-baseline-profile.zip", CreatedAt: at("2026-10-10T02:29:54Z")},
		{Name: "bun-linux-x64-musl-baseline.zip", CreatedAt: at("2026-10-10T02:30:07Z")},
		{Name: "bun-linux-x64-musl-profile.zip", CreatedAt: at("2026-10-10T02:30:20Z")},
		{Name: "bun-linux-x64-musl.zip", CreatedAt: at("2026-10-10T02:30:12Z")},
		{Name: "bun-linux-x64-profile.zip", CreatedAt: at("2026-10-10T02:29:58Z")},
		{Name: "bun-linux-x64.zip", CreatedAt: at("2026-10-10T02:30:00Z")},
		{Name: "bun-darwin-aarch64.zip", CreatedAt: at("2026-10-10T02:30:00Z")},
		{Name: "bun-windows-x64.zip", CreatedAt: at("2026-10-10T02:30:00Z")},
		{Name: "SHASUMS256.txt", CreatedAt: at("2026-10-10T02:30:40Z")},
	}

	got, err := Pick(assets, "bun", Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "bun-linux-x64.zip" {
		t.Fatalf("got %q, want %q", got.Name, "bun-linux-x64.zip")
	}
}

func TestPickRejectsForeignOS(t *testing.T) {
	tests := []struct {
		name   string
		assets []string
		want   string
	}{
		{
			name:   "android build next to host word",
			assets: []string{"tool-linux-amd64-android.tar.gz", "tool-linux-amd64.tar.gz"},
			want:   "tool-linux-amd64.tar.gz",
		},
		{
			name:   "android build is the only option",
			assets: []string{"tool-linux-amd64-android.tar.gz"},
			want:   "",
		},
		{
			name:   "ios build next to darwin word",
			assets: []string{"tool-macos-arm64-ios.tar.gz", "tool-macos-arm64.tar.gz"},
			want:   "tool-macos-arm64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := make([]Asset, 0, len(tt.assets))
			for _, n := range tt.assets {
				assets = append(assets, asset(n))
			}

			p := Platform{OS: "linux", Arch: "amd64"}
			if tt.name == "ios build next to darwin word" {
				p = Platform{OS: "darwin", Arch: "arm64"}
			}

			got, err := Pick(assets, "tool", p)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("expected error, got %q", got.Name)
				}
				return
			}
			if err != nil {
				t.Fatalf("Pick: %v", err)
			}
			if got.Name != tt.want {
				t.Fatalf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
}

// TestPickIgnoresCreatedAt checks the tie-break is deterministic and does not
// depend on upload order.
func TestPickIgnoresCreatedAt(t *testing.T) {
	p := Platform{OS: "linux", Arch: "amd64"}

	a := Asset{Name: "tool-linux-amd64.tar.gz", CreatedAt: at("2020-01-01T00:00:00Z")}
	b := Asset{Name: "tool-linux-amd64-extra.tar.gz", CreatedAt: at("2030-01-01T00:00:00Z")}

	got, err := Pick([]Asset{a, b}, "tool", p)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != a.Name {
		t.Fatalf("got %q, want %q", got.Name, a.Name)
	}

	b.CreatedAt, a.CreatedAt = a.CreatedAt, b.CreatedAt
	got, err = Pick([]Asset{a, b}, "tool", p)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != a.Name {
		t.Fatalf("got %q, want %q", got.Name, a.Name)
	}
}

func TestPickRejectsProfileAndDeprioritisesBaseline(t *testing.T) {
	p := Platform{OS: "linux", Arch: "amd64"}

	got, err := Pick([]Asset{asset("tool-linux-amd64-baseline.tar.gz"), asset("tool-linux-amd64.tar.gz")}, "tool", p)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "tool-linux-amd64.tar.gz" {
		t.Fatalf("got %q, want plain build", got.Name)
	}

	if _, err := Pick([]Asset{asset("tool-linux-amd64-profile.tar.gz")}, "tool", p); err == nil {
		t.Fatal("expected profile-only release to be rejected")
	}
}

// TestPickRepoNamedAfterOS ensures a repository whose name is an OS token does
// not disqualify its own assets.
func TestPickRepoNamedAfterOS(t *testing.T) {
	p := Platform{OS: "linux", Arch: "amd64"}

	got, err := Pick([]Asset{asset("windows-linux-amd64.tar.gz")}, "windows", p)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "windows-linux-amd64.tar.gz" {
		t.Fatalf("got %q", got.Name)
	}
}
