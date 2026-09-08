package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha256Hex(t *testing.T, content []byte) string {
	t.Helper()
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestParseChecksumHex_PlainSha256sum(t *testing.T) {
	got, err := parseChecksumHex("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08  Tarkov-Nexus_windows_amd64.zip\n")
	if err != nil {
		t.Fatalf("parseChecksumHex error: %v", err)
	}
	if got != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Errorf("parseChecksumHex = %q", got)
	}
}

func TestParseChecksumHex_CertutilFormat(t *testing.T) {
	// certutil -hashfile output: uppercase hash on its own line
	body := "SHA256 hash of file.zip:\n9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08\nCertUtil: -hashfile command completed successfully.\n"
	got, err := parseChecksumHex(body)
	if err != nil {
		t.Fatalf("parseChecksumHex error: %v", err)
	}
	if got != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Errorf("parseChecksumHex = %q", got)
	}
}

func TestParseChecksumHex_Malformed(t *testing.T) {
	for _, body := range []string{"", "not a checksum", "12345"} {
		if _, err := parseChecksumHex(body); err == nil {
			t.Errorf("parseChecksumHex(%q) expected error", body)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	payload := []byte("pretend this is a 90MB application zip")
	want := sha256Hex(t, payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(want + "  payload.zip\n"))
	}))
	defer srv.Close()

	path := writeTempFile(t, payload)

	if err := verifyChecksum(context.Background(), path, srv.URL); err != nil {
		t.Fatalf("verifyChecksum match case error: %v", err)
	}

	// Corrupt the file afterwards — verification must fail
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verifyChecksum(context.Background(), path, srv.URL)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("tampered file must fail with mismatch, got %v", err)
	}
}

func TestVerifyChecksum_FailClosedOnMissingChecksumFile(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	path := writeTempFile(t, []byte("payload"))
	err := verifyChecksum(context.Background(), path, srv.URL+"/missing.sha256")
	if err == nil || !strings.Contains(err.Error(), "not published") {
		t.Errorf("missing checksum file must fail closed, got %v", err)
	}
}

func TestVerifyChecksum_MalformedChecksumFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("oops, no hash here"))
	}))
	defer srv.Close()

	path := writeTempFile(t, []byte("payload"))
	if err := verifyChecksum(context.Background(), path, srv.URL); err == nil {
		t.Error("malformed checksum file must be an error, not a skip")
	}
}

func TestDownloadFile_ProgressAndContent(t *testing.T) {
	payload := []byte(strings.Repeat("x", 300*1024)) // 300KB, multiple chunks
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "307200")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "dl-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer tmp.Close()

	u := NewUpdater(consoleTestLogger{}, ChannelStable)
	if err := u.downloadFile(context.Background(), srv.URL, tmp); err != nil {
		t.Fatalf("downloadFile error: %v", err)
	}

	got, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) {
		t.Errorf("downloaded %d bytes, want %d", len(got), len(payload))
	}
	if u.status.DownloadProgress != 100 {
		t.Errorf("DownloadProgress = %d, want 100 after full download", u.status.DownloadProgress)
	}
}

// consoleTestLogger is a minimal logger for tests that exercise Updater methods.
type consoleTestLogger struct{}

func (consoleTestLogger) Debug(string)   {}
func (consoleTestLogger) Info(string)    {}
func (consoleTestLogger) Warning(string) {}
func (consoleTestLogger) Error(string)   {}
