package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func testReleaseNote(tag, body string, fetchedAt time.Time) ReleaseNote {
	return ReleaseNote{Tag: tag, Name: tag, Body: body, PublishedAt: &fetchedAt, ReleaseURL: "https://github.com/owner/repo/releases/tag/" + tag, FetchedAt: fetchedAt}
}

func TestReleaseNotesCache_PreservesEmptyRecordAndIgnoresCorruption(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.2.3")
	now := time.Now().UTC()
	note := testReleaseNote("v1.2.3", "  \n", now)
	service.mu.Lock()
	if !service.writeRecordLocked(note, note.Tag) {
		service.mu.Unlock()
		t.Fatal("empty record was not saved")
	}
	service.mu.Unlock()

	read := NewReleaseNotes(client, dir, "1.2.3").Read("1.2.3")
	if read.Note == nil || read.Note.Body != "  \n" || !read.Saved {
		t.Fatalf("empty record was not preserved: %+v", read)
	}
	if err := os.WriteFile(service.recordPath(note.Tag), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := NewReleaseNotes(client, dir, "1.2.3").Read("1.2.3")
	if corrupt.Note != nil || corrupt.Saved {
		t.Fatalf("corrupt record was accepted: %+v", corrupt)
	}
}

func TestReleaseNotesCache_WriteFailureKeepsFreshSessionAndSavedFile(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.2.3")
	now := time.Now().UTC()
	old := testReleaseNote("v1.2.3", "old", now.Add(-time.Hour))
	service.mu.Lock()
	if !service.writeRecordLocked(old, old.Tag) {
		service.mu.Unlock()
		t.Fatal("old record was not saved")
	}
	service.replaceNotesFile = func(string, string) error { return errors.New("replace failed") }
	newer := testReleaseNote("v1.2.3", "new", now)
	saved := service.writeRecordLocked(newer, newer.Tag)
	service.session[newer.Tag] = sessionReleaseNote{note: newer, saved: saved}
	service.mu.Unlock()
	if saved {
		t.Fatal("failed replacement reported a saved note")
	}
	session := service.Read("1.2.3")
	if session.Note == nil || session.Note.Body != "new" || session.Saved {
		t.Fatalf("fresh session note was lost or mislabeled: %+v", session)
	}
	restarted := NewReleaseNotes(client, dir, "1.2.3").Read("1.2.3")
	if restarted.Note == nil || restarted.Note.Body != "old" || !restarted.Saved {
		t.Fatalf("failed replacement damaged old record: %+v", restarted)
	}
}

func TestReleaseNotesCache_RollbackFailureKeepsValidReplacement(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.2.3")
	now := time.Now().UTC()
	old := testReleaseNote("v1.2.3", "old", now.Add(-time.Hour))
	fresh := testReleaseNote("v1.2.3", "fresh", now)
	service.mu.Lock()
	if !service.writeRecordLocked(fresh, fresh.Tag) {
		service.mu.Unlock()
		t.Fatal("fresh replacement was not saved")
	}
	oldRecord, err := json.Marshal(releaseNotesRecord{Schema: releaseNotesSchema, Repository: service.repository, Note: old})
	if err != nil {
		service.mu.Unlock()
		t.Fatal(err)
	}
	service.restoreNotesFile = func(string, string) error { return errors.New("rollback replace failed") }
	restored := service.restoreRecordLocked(service.recordPath(fresh.Tag), oldRecord, 0o600, true)
	service.mu.Unlock()
	if restored {
		t.Fatal("failed rollback reported success")
	}
	restarted := NewReleaseNotes(client, dir, "1.2.3").Read("1.2.3")
	if restarted.Note == nil || restarted.Note.Body != "fresh" || !restarted.Saved {
		t.Fatalf("rollback failure deleted the valid replacement: %+v", restarted)
	}
}

func TestReleaseNotesCache_UnusableDestinationDegradesToSession(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "release-notes"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewReleaseNotes(client, dir, "1.2.3")
	published := time.Now().UTC()
	result := service.RememberOffer(UpdateInfo{Version: "1.2.3", ReleaseURL: "https://github.com/owner/repo/releases/tag/v1.2.3", ReleaseDate: published, ReleaseBody: "fresh"})
	if result.Note == nil || result.Note.Body != "fresh" || result.Saved {
		t.Fatalf("unusable cache blocked or mislabeled fresh content: %+v", result)
	}
}

func TestReleaseNotesCache_RejectsRedirectedCacheDirectory(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dataDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, "release-notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dataDir, "release-notes", "v1")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	service := NewReleaseNotes(client, dataDir, "1.2.3")
	published := time.Now().UTC()
	result := service.RememberOffer(UpdateInfo{Version: "1.2.3", ReleaseURL: "https://github.com/owner/repo/releases/tag/v1.2.3", ReleaseDate: published, ReleaseBody: "fresh"})
	if result.Note == nil || result.Note.Body != "fresh" || result.Saved {
		t.Fatalf("redirected cache did not degrade to session state: %+v", result)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("redirected cache wrote %d file(s) outside the data directory", len(entries))
	}
}

func TestReleaseNotesCache_ClassifiesInvalidUTF8AsUnusable(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
	if _, ok := service.usableCacheDir(true); !ok {
		t.Fatal("cache directory unavailable")
	}
	tag := "v2.0.0"
	crafted := releaseNotesRecord{
		Schema: releaseNotesSchema, Repository: service.repository,
		Note: testReleaseNote(tag, "crafted", time.Now().UTC()),
	}
	data, err := json.Marshal(crafted)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("crafted"), []byte{0xff}, 1)
	if err := os.WriteFile(service.recordPath(tag), data, 0o600); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	records := service.recordFilesLocked()
	service.mu.Unlock()
	if len(records) != 1 || records[0].tag != "" {
		t.Fatalf("invalid UTF-8 record was classified as usable: %+v", records)
	}
	read := service.Read("2.0.0")
	if read.Note != nil || read.Saved {
		t.Fatalf("invalid UTF-8 record was readable: %+v", read)
	}
}

func TestReleaseNotesCache_EnforcesCountAndByteBoundsWithoutDeletingProtectedRecords(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.0.0")
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < maxNoteRecords+6; i++ {
		version := "1.0." + strconv.Itoa(i)
		tag := "v" + version
		note := testReleaseNote(tag, "body", base.Add(time.Duration(i)*time.Minute))
		service.mu.Lock()
		service.lastOfferedTag = tag
		if !service.writeRecordLocked(note, tag) {
			service.mu.Unlock()
			t.Fatalf("record %d was not saved", i)
		}
		service.mu.Unlock()
	}
	files, err := filepath.Glob(filepath.Join(service.cacheDir(), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > maxNoteRecords {
		t.Fatalf("cache retained %d records", len(files))
	}
	if NewReleaseNotes(client, dir, "1.0.0").Read("1.0.0").Note == nil {
		t.Fatal("installed version was pruned")
	}
	service.mu.Lock()
	validBefore := make(map[string]struct{})
	for _, record := range service.recordFilesLocked() {
		if record.tag != "" {
			validBefore[record.tag] = struct{}{}
		}
	}
	service.mu.Unlock()

	for i := 0; i < 9; i++ {
		path := filepath.Join(service.cacheDir(), "oversize-"+strconv.Itoa(i)+".json")
		file, createErr := os.Create(path)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if truncateErr := file.Truncate(maxNoteRecordBytes + 1); truncateErr != nil {
			_ = file.Close()
			t.Fatal(truncateErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	note := testReleaseNote("v2.0.0", "bounded", time.Now().UTC())
	service.mu.Lock()
	if !service.writeRecordLocked(note, note.Tag) {
		service.mu.Unlock()
		t.Fatal("cache could not reclaim unprotected oversized records")
	}
	var total int64
	retained := 0
	for _, record := range service.recordFilesLocked() {
		total += record.size
		if _, existed := validBefore[record.tag]; existed {
			retained++
		}
	}
	service.mu.Unlock()
	if total > maxNoteCacheBytes {
		t.Fatalf("cache retained %d bytes", total)
	}
	if retained < len(validBefore)-1 {
		t.Fatalf("junk files evicted valid notes first: retained %d of %d", retained, len(validBefore))
	}
}

func TestReleaseNotesCache_PruneFailureKeepsFreshNoteSessionOnly(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.0.0")
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < maxNoteRecords; i++ {
		tag := "v1.0." + strconv.Itoa(i)
		note := testReleaseNote(tag, "body", base.Add(time.Duration(i)*time.Minute))
		service.mu.Lock()
		if !service.writeRecordLocked(note, tag) {
			service.mu.Unlock()
			t.Fatalf("record %d was not saved", i)
		}
		service.mu.Unlock()
	}

	blockedPath := service.recordPath("v1.0.1")
	service.removeNotesFile = func(path string) error {
		if path == blockedPath {
			return errors.New("remove failed")
		}
		return os.Remove(path)
	}
	published := time.Now().UTC()
	result := service.RememberOffer(UpdateInfo{
		Version:     "2.0.0",
		ReleaseURL:  "https://github.com/owner/repo/releases/tag/v2.0.0",
		ReleaseDate: published,
		ReleaseName: "Offer",
		ReleaseBody: "fresh",
	})
	if result.Note == nil || result.Note.Body != "fresh" || result.Saved {
		t.Fatalf("prune failure did not retain an unsaved session note: %+v", result)
	}
	if _, err := os.Stat(service.recordPath("v2.0.0")); !os.IsNotExist(err) {
		t.Fatalf("failed retention left the new record on disk: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(service.cacheDir(), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != maxNoteRecords {
		t.Fatalf("cache retained %d records after failed pruning", len(files))
	}
	session := service.Read("2.0.0")
	if session.Note == nil || session.Note.Body != "fresh" || session.Saved {
		t.Fatalf("fresh note was not kept session-only: %+v", session)
	}
	restarted := NewReleaseNotes(client, dir, "1.0.0").Read("2.0.0")
	if restarted.Note != nil || restarted.Saved {
		t.Fatalf("failed retention persisted the new note: %+v", restarted)
	}
}

func TestReleaseNotesCache_PartialPruneFailureRestoresEveryDeletedRecord(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "9.9.9")
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < maxNoteRecords; i++ {
		tag := "v1.0." + strconv.Itoa(i)
		note := testReleaseNote(tag, "body", base.Add(time.Duration(i)*time.Minute))
		service.mu.Lock()
		if !service.writeRecordLocked(note, tag) {
			service.mu.Unlock()
			t.Fatalf("record %d was not saved", i)
		}
		service.mu.Unlock()
	}
	duplicate := filepath.Join(service.cacheDir(), "duplicate.json")
	oldest := service.recordPath("v1.0.0")
	data, err := os.ReadFile(oldest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(duplicate, data, 0o600); err != nil {
		t.Fatal(err)
	}
	before := make(map[string][]byte)
	entries, err := os.ReadDir(service.cacheDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		content, readErr := os.ReadFile(filepath.Join(service.cacheDir(), entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		before[entry.Name()] = content
	}
	targetPath := service.recordPath("v2.0.0")
	service.removeNotesFile = func(path string) error {
		if path == targetPath {
			return os.Remove(path)
		}
		if path == oldest {
			return errors.New("valid record remove failed")
		}
		return os.Remove(path)
	}
	result := service.RememberOffer(UpdateInfo{
		Version: "2.0.0", ReleaseURL: "https://github.com/owner/repo/releases/tag/v2.0.0",
		ReleaseDate: time.Now().UTC(), ReleaseBody: "fresh",
	})
	if result.Note == nil || result.Saved {
		t.Fatalf("partial prune did not fail session-only: %+v", result)
	}
	for name, expected := range before {
		actual, readErr := os.ReadFile(filepath.Join(service.cacheDir(), name))
		if readErr != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("record %s was not restored: err=%v", name, readErr)
		}
	}
	if _, err := os.Stat(service.recordPath("v2.0.0")); !os.IsNotExist(err) {
		t.Fatalf("failed target remained on disk: %v", err)
	}
}
