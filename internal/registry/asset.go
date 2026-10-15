package registry

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// osAliases maps a GOOS value to tokens commonly found in asset names.
var osAliases = map[string][]string{
	"linux":   {"linux"},
	"darwin":  {"darwin", "macos", "osx", "apple"},
	"windows": {"windows", "win32", "win64", "win"},
	"freebsd": {"freebsd"},
	"netbsd":  {"netbsd"},
	"openbsd": {"openbsd"},
}

// knownOS maps every operating-system token oir understands to the GOOS value
// it belongs to. It lets scoreAsset reject assets that advertise a different
// OS even when they also mention the requested one, for example Bun's Android
// builds named "bun-linux-x64-android.zip".
var knownOS = map[string]string{
	"linux":   "linux",
	"darwin":  "darwin",
	"macos":   "darwin",
	"osx":     "darwin",
	"apple":   "darwin",
	"windows": "windows",
	"win32":   "windows",
	"win64":   "windows",
	"win":     "windows",
	"freebsd": "freebsd",
	"netbsd":  "netbsd",
	"openbsd": "openbsd",
	"android": "android",
	"ios":     "ios",
	"solaris": "solaris",
	"illumos": "illumos",
	"wasm":    "wasm",
}

// archAliases maps a GOARCH value to tokens commonly found in asset names.
var archAliases = map[string][]string{
	"amd64": {"amd64", "x86 64", "x64"},
	"arm64": {"arm64", "aarch64"},
	"386":   {"386", "i386", "i686"},
	"arm":   {"armv7", "armv6", "armhf", "arm"},
}

// archiveExtensions are the archive formats oir can extract. Assets without
// one of these are treated as raw single-file binaries, which many projects
// publish, for example "tool-1.2.3-linux-amd64".
var archiveExtensions = []string{".tar.gz", ".tgz", ".zip"}

// excludedTokens mark assets that are not the tool binary itself.
var excludedTokens = []string{
	"checksum", "checksums", "sha256", "sha512", "sha1", "md5", "sums",
	"sig", "asc", "pem", "sbom", "spdx", "provenance", "attestation",
	// Instrumented builds are not the binary users want to run.
	"profile",
}

// excludedSuffixes mark assets that are not extractable archives.
var excludedSuffixes = []string{
	".txt", ".json", ".yml", ".yaml", ".md",
	".sig", ".asc", ".pem", ".sbom", ".spdx",
	".deb", ".rpm", ".apk", ".dmg", ".pkg", ".msi",
	".xz", ".zst", ".bz2", ".7z", ".tar", ".gz",
}

// variantTokens are tolerated but deprioritised, so a plain build wins.
var variantTokens = []string{"musl", "static", "gnu", "gnueabihf", "glibc", "debug", "nocgo", "baseline"}

// Pick chooses the best release asset for the given repo and platform.
func Pick(assets []Asset, repo string, p Platform) (Asset, error) {
	best, bestAsset := -1, Asset{}
	found := false

	for _, a := range assets {
		score, ok := scoreAsset(a.Name, repo, p)
		if !ok {
			continue
		}
		if !found || better(a, score, bestAsset, best) {
			best, bestAsset, found = score, a, true
		}
	}

	if !found {
		return Asset{}, fmt.Errorf("no asset matches %s/%s among %d assets: %s",
			p.OS, p.Arch, len(assets), assetNames(assets))
	}

	return bestAsset, nil
}

// better reports whether candidate a should replace the current best b.
// Higher score wins. Ties break deterministically: the shorter asset name
// wins, so a plain build beats suffixed variants, then lexical order. Upload
// time is deliberately ignored: it is not a platform signal and previously
// made oir prefer cross-target builds such as bun-linux-x64-android.zip.
func better(a Asset, aScore int, b Asset, bScore int) bool {
	if aScore != bScore {
		return aScore > bScore
	}
	if len(a.Name) != len(b.Name) {
		return len(a.Name) < len(b.Name)
	}

	return a.Name < b.Name
}

func scoreAsset(name, repo string, p Platform) (int, bool) {
	norm := normalize(name)
	repoNorm := normalize(repo)
	lower := strings.ToLower(name)
	archived := hasAnySuffix(lower, archiveExtensions)

	// Reject known non-binary formats. This only applies to assets that are
	// not archives, so ".tar.gz" is not tripped up by the ".gz" suffix.
	if !archived && hasAnySuffix(lower, excludedSuffixes) {
		return 0, false
	}
	for _, tok := range excludedTokens {
		if containsToken(norm, tok) {
			return 0, false
		}
	}
	// Reject cross-target builds that mention another OS next to ours, for
	// example "bun-linux-x64-android.zip".
	if foreignOS(norm, repoNorm, p.OS) {
		return 0, false
	}
	if !matchesAlias(norm, osAliases[p.OS]) {
		return 0, false
	}
	if !matchesAlias(norm, archAliases[p.Arch]) {
		return 0, false
	}

	// Prefer archives, which may bundle docs and completions, over raw
	// single-file binaries when a release publishes both.
	score := 0
	if archived {
		score += 10
	}
	if containsToken(norm, repoNorm) {
		score += 100
	}
	score += 20 // os matched
	score += 20 // arch matched

	for _, tok := range variantTokens {
		if containsToken(norm, tok) {
			score -= 50
		}
	}

	return score, true
}

// normalize lowercases a string and turns separators into spaces so that
// tokens like "x86_64" and "x86-64" compare equal to the alias "x86 64".
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', '+', '(', ')', '[', ']':
			return ' '
		}
		return r
	}, s)

	return strings.Join(strings.Fields(s), " ")
}

func containsToken(norm, token string) bool {
	if token == "" {
		return false
	}

	return strings.Contains(" "+norm+" ", " "+token+" ")
}

func matchesAlias(norm string, aliases []string) bool {
	for _, a := range aliases {
		if containsToken(norm, a) {
			return true
		}
	}

	return false
}

// foreignOS reports whether the asset advertises an OS other than want. Tokens
// that are part of the repository name are ignored, so a repo called "apple"
// or "windows" does not disqualify its own linux asset.
func foreignOS(norm, repoNorm, want string) bool {
	repoTokens := strings.Fields(repoNorm)

	for _, tok := range strings.Fields(norm) {
		os, known := knownOS[tok]
		if !known || os == want || slices.Contains(repoTokens, tok) {
			continue
		}

		return true
	}

	return false
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}

	return false
}

func assetNames(assets []Asset) string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	if len(names) > 10 {
		names = append(names[:10], "...")
	}

	return strings.Join(names, ", ")
}
