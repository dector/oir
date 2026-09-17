package spec

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    Spec
		wantErr bool
	}{
		{in: "gh:dector/ror", want: Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}},
		{in: "github:dector/ror", want: Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}},
		{in: "dector/ror", want: Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}},
		{in: "gh:dector/ror@v1.2.3", want: Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror", Version: "v1.2.3"}},
		{in: "gh:neovim/neovim@stable", want: Spec{Backend: BackendGitHub, Owner: "neovim", Repo: "neovim", Version: "stable"}},
		{in: "  gh:dector/ror  ", want: Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}},
		{in: "", wantErr: true},
		{in: "gh:dector", wantErr: true},
		{in: "gh:/ror", wantErr: true},
		{in: "gh:dector/", wantErr: true},
		{in: "gh:dector/ror/extra", wantErr: true},
		{in: "npm:left-pad", wantErr: true},
	}

	for _, tt := range tests {
		got, err := Parse(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q): expected error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestKeyAndString(t *testing.T) {
	sp := Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}
	if got, want := sp.Key(), "github/dector/ror"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := sp.String(), "github:dector/ror"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	sp.Version = "v1"
	if got, want := sp.String(), "github:dector/ror@v1"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestFromKey(t *testing.T) {
	sp, err := FromKey("github/dector/ror")
	if err != nil {
		t.Fatalf("FromKey: %v", err)
	}
	want := Spec{Backend: BackendGitHub, Owner: "dector", Repo: "ror"}
	if sp != want {
		t.Errorf("FromKey = %+v, want %+v", sp, want)
	}
	if sp.Version != "" {
		t.Errorf("FromKey version = %q, want empty", sp.Version)
	}
}

func TestFromKeyRoundTrip(t *testing.T) {
	in := Spec{Backend: BackendGitHub, Owner: "BurntSushi", Repo: "Ripgrep"}
	out, err := FromKey(in.Key())
	if err != nil {
		t.Fatalf("FromKey: %v", err)
	}
	if out.Key() != in.Key() {
		t.Errorf("round trip key = %q, want %q", out.Key(), in.Key())
	}
}

func TestFromKeyInvalid(t *testing.T) {
	for _, in := range []string{"", "github", "github/only", "github//repo", "npm:left-pad", "github/o/tool/extra"} {
		if _, err := FromKey(in); err == nil {
			t.Errorf("FromKey(%q): expected error", in)
		}
	}
}

func TestKeyLowercasesButStringPreservesCase(t *testing.T) {
	sp := Spec{Backend: BackendGitHub, Owner: "BurntSushi", Repo: "Ripgrep"}

	if got, want := sp.Key(), "github/burntsushi/ripgrep"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := sp.String(), "github:BurntSushi/Ripgrep"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
