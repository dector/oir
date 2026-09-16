package archive

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ignoredDirNames hold documentation and data, never the tool binary.
var ignoredDirNames = map[string]bool{
	"doc": true, "docs": true, "man": true, "share": true, "completions": true,
	"completion": true, "shell": true, ".github": true, "licenses": true,
	"license": true, "examples": true, "testdata": true,
}

// ignoredFileNames are never the tool binary.
var ignoredFileNames = map[string]bool{
	"license": true, "licence": true, "notice": true, "readme": true,
	"changelog": true, "changes": true, "authors": true, "contributors": true,
	"install": true, "makefile": true,
}

// FindBinary locates the tool binary inside an extracted tree.
//
// repo is the repository name, used as a strong hint (e.g. "ror"). The
// search prefers a file named after the repo, then any executable file.
func FindBinary(root, repo string) (string, error) {
	type candidate struct {
		path  string
		rel   string
		score int
	}

	var found []candidate

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && ignoredDirNames[strings.ToLower(d.Name())] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}

		score := scoreBinary(rel, d.Name(), repo, info.Mode())
		if score < 0 {
			return nil
		}
		found = append(found, candidate{path: p, rel: rel, score: score})

		return nil
	})
	if err != nil {
		return "", err
	}

	if len(found) == 0 {
		return "", fmt.Errorf("no tool binary found in archive")
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return len(found[i].rel) < len(found[j].rel)
	})

	return found[0].path, nil
}

func scoreBinary(rel, name, repo string, mode fs.FileMode) int {
	base := strings.TrimSuffix(name, ".exe")
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	lowerBase := strings.ToLower(base)
	lowerStem := strings.ToLower(stem)

	if strings.HasPrefix(base, ".") {
		return -1
	}
	if ignoredFileNames[lowerStem] {
		return -1
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".md", ".txt", ".1", ".json", ".yml", ".yaml", ".toml", ".cfg", ".sh", ".ps1", ".bat":
		return -1
	}

	score := 0
	switch {
	case lowerBase == strings.ToLower(repo):
		score += 100
	case strings.HasPrefix(lowerBase, strings.ToLower(repo)):
		score += 60
	}
	if filepath.Dir(rel) == "." {
		score += 20
	}
	if mode&0o111 != 0 {
		score += 30
	}
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(name), ".exe") {
		score += 10
	}

	return score
}
