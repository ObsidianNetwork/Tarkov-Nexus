package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	checksumSuffix        = ".sha256"
	checksumRequestTimout = 15 * time.Second
	checksumBodyLimit     = 1 << 20 // checksum files are tiny; refuse absurd payloads
)

// verifyChecksum fetches the checksum file published next to the release
// asset and compares it against the SHA256 of the downloaded file.
// Fail-closed: a missing or unparsable checksum file is an error.
func verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	ctx, cancel := context.WithTimeout(ctx, checksumRequestTimout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return fmt.Errorf("build checksum request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("checksum file not published: %s", checksumURL)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch checksum: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, checksumBodyLimit))
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	want, err := parseChecksumHex(string(body))
	if err != nil {
		return err
	}

	got, err := fileSHA256(filePath)
	if err != nil {
		return fmt.Errorf("hash downloaded file: %w", err)
	}

	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

// fileSHA256 returns the hex-encoded SHA256 of a file's contents.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
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

// parseChecksumHex extracts the expected hash from checksum file contents.
// Tolerates both plain "sha256sum" output (hash + filename) and Windows
// certutil output ("SHA256 hash of file:\n<hash>\nCertUtil: ...").
func parseChecksumHex(body string) (string, error) {
	for _, field := range strings.Fields(body) {
		f := strings.ToLower(strings.Trim(field, ",:"))
		if len(f) == 64 && isHex(f) {
			return f, nil
		}
	}
	return "", fmt.Errorf("no sha256 hash found in checksum file")
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
