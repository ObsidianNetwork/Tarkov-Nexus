package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"
)

const releaseNotesSchema = 1
const maxNoteRecordBytes = 8 << 20
const maxNoteRecords = 32
const maxNoteCacheBytes = 64 << 20

type releaseNotesRecord struct {
	Schema     int         `json:"schema"`
	Repository string      `json:"repository"`
	Note       ReleaseNote `json:"note"`
}

type noteRecordFile struct {
	path      string
	tag       string
	size      int64
	fetchedAt time.Time
}

type noteRecordBackup struct {
	file noteRecordFile
	data []byte
	mode os.FileMode
}

func (n *ReleaseNotes) cacheDir() string {
	if n.dataDir == "" {
		return ""
	}
	return filepath.Join(n.dataDir, "release-notes", "v1")
}

func (n *ReleaseNotes) recordPath(tag string) string {
	sum := sha256.Sum256([]byte(n.repository + "\n" + tag))
	return filepath.Join(n.cacheDir(), hex.EncodeToString(sum[:])+".json")
}

func (n *ReleaseNotes) usableCacheDir(create bool) (string, bool) {
	dir := n.cacheDir()
	if dir == "" {
		return "", false
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", false
		}
	}
	current := n.dataDir
	for _, name := range []string{"release-notes", "v1"} {
		current = filepath.Join(current, name)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", false
		}
	}
	return dir, true
}

func regularRecord(path string) (os.FileInfo, bool) {
	info, err := os.Lstat(path)
	return info, err == nil && info.Mode().IsRegular()
}

func (n *ReleaseNotes) validateNote(note ReleaseNote, tag string) error {
	if note.Tag != tag || note.ReleaseURL != n.client.notesURL(tag) || note.FetchedAt.IsZero() || len(note.Body) > maxNoteBodyBytes || !utf8.ValidString(note.Body) {
		return errors.New("invalid cached release note")
	}
	return nil
}

func (n *ReleaseNotes) readRecordLocked(tag string) (ReleaseNote, bool) {
	if _, ok := n.usableCacheDir(false); !ok {
		return ReleaseNote{}, false
	}
	path := n.recordPath(tag)
	if _, ok := regularRecord(path); !ok {
		return ReleaseNote{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return ReleaseNote{}, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxNoteRecordBytes+1))
	if err != nil || len(data) > maxNoteRecordBytes || !utf8.Valid(data) {
		return ReleaseNote{}, false
	}
	var record releaseNotesRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Schema != releaseNotesSchema || record.Repository != n.repository || n.validateNote(record.Note, tag) != nil {
		return ReleaseNote{}, false
	}
	return record.Note, true
}

func (n *ReleaseNotes) writeRecordLocked(note ReleaseNote, tag string) bool {
	dir := n.cacheDir()
	if dir == "" || n.validateNote(note, tag) != nil {
		return false
	}
	record := releaseNotesRecord{Schema: releaseNotesSchema, Repository: n.repository, Note: note}
	data, err := json.Marshal(record)
	if err != nil || len(data) > maxNoteRecordBytes {
		return false
	}
	if _, ok := n.usableCacheDir(true); !ok {
		return false
	}
	if !n.canRetainLocked(tag, int64(len(data))) {
		return false
	}
	targetPath := n.recordPath(tag)
	previous, previousMode, previousExists, err := readRecordBackup(targetPath)
	if err != nil {
		return false
	}
	temp, err := os.CreateTemp(dir, ".release-notes-*.tmp")
	if err != nil {
		return false
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil || writeAndSync(temp, data) != nil {
		_ = temp.Close()
		return false
	}
	if err := temp.Close(); err != nil {
		return false
	}
	if err := n.replaceNotesFile(tempPath, targetPath); err != nil {
		n.restoreRecordLocked(targetPath, previous, previousMode, previousExists)
		return false
	}
	if !n.pruneLocked(tag) {
		n.restoreRecordLocked(targetPath, previous, previousMode, previousExists)
		return false
	}
	return true
}

func readRecordBackup(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, 0, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxNoteRecordBytes {
		return nil, 0, false, errors.New("cannot back up release note record")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode().Perm(), true, nil
}

func (n *ReleaseNotes) restoreRecordLocked(path string, data []byte, mode os.FileMode, existed bool) bool {
	if !existed {
		err := n.removeNotesFile(path)
		return err == nil || os.IsNotExist(err)
	}
	temp, err := os.CreateTemp(n.cacheDir(), ".release-notes-rollback-*.tmp")
	if err != nil {
		return false
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(mode); err != nil || writeAndSync(temp, data) != nil || temp.Close() != nil || n.restoreNotesFile(tempPath, path) != nil {
		_ = temp.Close()
		return false
	}
	return true
}

func writeAndSync(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func (n *ReleaseNotes) recordFilesLocked() []noteRecordFile {
	dir, ok := n.usableCacheDir(false)
	if !ok {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]noteRecordFile, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		item := noteRecordFile{path: path, size: info.Size(), fetchedAt: info.ModTime()}
		file, err := os.Open(path)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, maxNoteRecordBytes+1))
			_ = file.Close()
			if readErr == nil && len(data) <= maxNoteRecordBytes && utf8.Valid(data) {
				var record releaseNotesRecord
				if json.Unmarshal(data, &record) == nil && record.Schema == releaseNotesSchema && record.Repository == n.repository &&
					n.validateNote(record.Note, record.Note.Tag) == nil && path == n.recordPath(record.Note.Tag) {
					item.tag = record.Note.Tag
					item.fetchedAt = record.Note.FetchedAt
				}
			}
		}
		files = append(files, item)
	}
	sort.SliceStable(files, func(i, j int) bool {
		if (files[i].tag == "") != (files[j].tag == "") {
			return files[i].tag == ""
		}
		return files[i].fetchedAt.Before(files[j].fetchedAt)
	})
	return files
}

func (n *ReleaseNotes) protectedTag(tag, current string) bool {
	return tag != "" && (tag == current || tag == n.installedTag || tag == n.lastOfferedTag)
}

func (n *ReleaseNotes) canRetainLocked(tag string, newSize int64) bool {
	files := n.recordFilesLocked()
	count := 1
	total := newSize
	for _, file := range files {
		if file.path == n.recordPath(tag) {
			continue
		}
		count++
		total += file.size
	}
	if count <= maxNoteRecords && total <= maxNoteCacheBytes {
		return true
	}
	for _, file := range files {
		if file.path == n.recordPath(tag) || n.protectedTag(file.tag, tag) {
			continue
		}
		count--
		total -= file.size
		if count <= maxNoteRecords && total <= maxNoteCacheBytes {
			return true
		}
	}
	return false
}

func (n *ReleaseNotes) pruneLocked(currentTag string) bool {
	files := n.recordFilesLocked()
	var total int64
	for _, file := range files {
		total += file.size
	}
	backups := make([]noteRecordBackup, 0)
	type stagedJunkRecord struct {
		original string
		staged   string
	}
	stagedJunk := make([]stagedJunkRecord, 0)
	restoreStagedJunk := func() {
		for i := len(stagedJunk) - 1; i >= 0; i-- {
			_ = os.Rename(stagedJunk[i].staged, stagedJunk[i].original)
		}
	}
	for len(files) > maxNoteRecords || total > maxNoteCacheBytes {
		index := -1
		for i, file := range files {
			if !n.protectedTag(file.tag, currentTag) {
				index = i
				break
			}
		}
		if index < 0 {
			restoreStagedJunk()
			return false
		}
		file := files[index]
		if file.tag == "" {
			temp, err := os.CreateTemp(n.cacheDir(), ".release-notes-prune-*.tmp")
			if err != nil {
				restoreStagedJunk()
				return false
			}
			stagedPath := temp.Name()
			if err := temp.Close(); err != nil {
				_ = os.Remove(stagedPath)
				restoreStagedJunk()
				return false
			}
			if err := os.Remove(stagedPath); err != nil {
				restoreStagedJunk()
				return false
			}
			if err := os.Rename(file.path, stagedPath); err != nil {
				_ = os.Remove(stagedPath)
				restoreStagedJunk()
				return false
			}
			stagedJunk = append(stagedJunk, stagedJunkRecord{original: file.path, staged: stagedPath})
			total -= file.size
			files = append(files[:index], files[index+1:]...)
			continue
		}
		data, mode, exists, err := readRecordBackup(file.path)
		if err != nil || !exists {
			restoreStagedJunk()
			return false
		}
		backups = append(backups, noteRecordBackup{file: file, data: data, mode: mode})
		total -= file.size
		files = append(files[:index], files[index+1:]...)
	}
	removed := make([]noteRecordBackup, 0, len(backups))
	for _, backup := range backups {
		if n.removeNotesFile(backup.file.path) != nil {
			for i := len(removed) - 1; i >= 0; i-- {
				n.restoreRecordLocked(removed[i].file.path, removed[i].data, removed[i].mode, true)
			}
			restoreStagedJunk()
			return false
		}
		removed = append(removed, backup)
	}
	for _, junk := range stagedJunk {
		_ = os.Remove(junk.staged)
	}
	return true
}
