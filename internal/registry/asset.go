package registry

import (
	"fmt"
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

// archAliases maps a GOARCH value to tokens commonly found in asset names.
var archAliases = map[string][]string{
	"amd64": {"amd64", "x86 64", "x64"},
	"arm64": {"arm64", "aarch64"},
	"386":   {"386", "i386", "i686"},
	"arm":   {"armv7", "armv6", "armhf", "arm"},
}

// archiveExtensions are the archive formats oir can extract.
var archiveExtensions = []string{".tar.gz", ".tgz", ".zip"}

// excludedTokens mark assets that are not the tool binary itself.
var excludedTokens = []string{
	"checksum", "checksums", "sha256", "sha512", "sha1", "md5", "sums",
	"sig", "asc", "pem", "sbom", "spdx", "provenance", "attestation",
}

// excludedSuffixes mark assets that are not extractable archives.
var excludedSuffixes = []string{
	".txt", ".json", ".yml", ".yaml", ".md",
	".sig", ".asc", ".pem", ".sbom", ".spdx",
	".deb", ".rpm", ".apk", ".dmg", ".pkg", ".msi",
	".xz", ".zst", ".bz2", ".7z", ".tar",
}

// variantTokens are tolerated but deprioritised, so a plain build wins.
var variantTokens = []string{"musl", "static", "gnu", "gnueabihf", "glibc", "debug", "nocgo"}

// Pick chooses the best release asset for the given repo and platform.
func Pick(assets []Asset, repo string, p Platform) (Asset, error) {
	best, bestAsset := -1, Asset{}

	for _, a := range assets {
		score, ok := scoreAsset(a.Name, repo, p)
		if !ok {
			continue
		}
		if score > best || (score == best && a.CreatedAt.After(bestAsset.CreatedAt)) {
			best, bestAsset = score, a
		}
	}

	if best < 0 {
		return Asset{}, fmt.Errorf("no asset matches %s/%s among %d assets: %s",
			p.OS, p.Arch, len(assets), assetNames(assets))
	}

	return bestAsset, nil
}

func scoreAsset(name, repo string, p Platform) (int, bool) {
	norm := normalize(name)

	if !hasAnySuffix(strings.ToLower(name), archiveExtensions) {
		return 0, false
	}
	if hasAnySuffix(strings.ToLower(name), excludedSuffixes) {
		return 0, false
	}
	for _, tok := range excludedTokens {
		if containsToken(norm, tok) {
			return 0, false
		}
	}
	if !matchesAlias(norm, osAliases[p.OS]) {
		return 0, false
	}
	if !matchesAlias(norm, archAliases[p.Arch]) {
		return 0, false
	}

	score := 10 // has an extractable archive extension
	if containsToken(norm, normalize(repo)) {
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
