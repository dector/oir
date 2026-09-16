// Package verify checks downloaded files against published checksums.
package verify

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// SHA256File returns the lowercase hex sha256 of the file at p.
func SHA256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// CheckSHA256 compares the file at p with a digest. The digest may be given
// as "sha256:<hex>" or as a bare hex string.
func CheckSHA256(p, digest string) error {
	want := strings.ToLower(strings.TrimSpace(digest))
	want = strings.TrimPrefix(want, "sha256:")
	if want == "" {
		return fmt.Errorf("empty sha256 digest")
	}

	got, err := SHA256File(p)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch:\n  expected sha256:%s\n  actual   sha256:%s", want, got)
	}

	return nil
}

// FindChecksum scans a checksums file for the entry matching filename and
// returns its sha256 hex digest.
//
// Supported line formats:
//
//	<hex>  <filename>
//	<hex> *<filename>
//	SHA256 (<filename>) = <hex>
func FindChecksum(r io.Reader, filename string) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	for scanner.Scan() {
		name, digest, ok := parseChecksumLine(scanner.Text())
		if ok && name == filename {
			return digest, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("no checksum entry for %q", filename)
}

func parseChecksumLine(line string) (name, digest string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	// BSD style: SHA256 (file) = hex
	if strings.HasPrefix(line, "SHA256 (") {
		end := strings.Index(line, ")")
		if end < 0 {
			return "", "", false
		}
		name = strings.TrimSpace(line[len("SHA256 ("):end])
		rest := strings.TrimSpace(line[end+1:])
		rest = strings.TrimPrefix(rest, "=")
		digest = strings.TrimSpace(rest)
		if name == "" || !isHex(digest) {
			return "", "", false
		}
		return path.Base(name), strings.ToLower(digest), true
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	digest = fields[0]
	if !isHex(digest) {
		return "", "", false
	}
	name = strings.TrimPrefix(fields[len(fields)-1], "*")

	return path.Base(name), strings.ToLower(digest), true
}

func isHex(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)

	return err == nil
}
