package updater

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

type NoteLookup string

const (
	NoteUnchecked NoteLookup = "unchecked"
	NotePublished NoteLookup = "published"
	NoteMissing   NoteLookup = "missing"
	NoteFailed    NoteLookup = "failed"
)

type ReleaseNote struct {
	Tag         string     `json:"tag"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	PublishedAt *time.Time `json:"publishedAt"`
	ReleaseURL  string     `json:"releaseUrl"`
	FetchedAt   time.Time  `json:"fetchedAt"`
}

type ReleaseNotesResult struct {
	Version    string       `json:"version"`
	Tag        string       `json:"tag"`
	Lookup     NoteLookup   `json:"lookup"`
	Note       *ReleaseNote `json:"note"`
	Saved      bool         `json:"saved"`
	ReleaseURL string       `json:"releaseUrl"`
	RetryAt    *time.Time   `json:"retryAt"`
}

type ReleaseNotes struct {
	client           *ReleaseClient
	dataDir          string
	repository       string
	installedTag     string
	mu               sync.Mutex
	generations      map[string]uint64
	results          map[string]ReleaseNotesResult
	session          map[string]sessionReleaseNote
	lastOfferedTag   string
	replaceNotesFile func(string, string) error
	restoreNotesFile func(string, string) error
	removeNotesFile  func(string) error
}

type sessionReleaseNote struct {
	note  ReleaseNote
	saved bool
}

func NewReleaseNotes(client *ReleaseClient, dataDir string, installedVersion string) *ReleaseNotes {
	installedTag, _ := noteTag(installedVersion)
	return &ReleaseNotes{
		client:           client,
		dataDir:          dataDir,
		repository:       client.owner + "/" + client.repo,
		installedTag:     installedTag,
		generations:      make(map[string]uint64),
		results:          make(map[string]ReleaseNotesResult),
		session:          make(map[string]sessionReleaseNote),
		replaceNotesFile: replaceNotesFile,
		restoreNotesFile: replaceNotesFile,
		removeNotesFile:  os.Remove,
	}
}

func (n *ReleaseNotes) Read(version string) ReleaseNotesResult {
	result := ReleaseNotesResult{Version: strings.TrimSpace(version), Lookup: NoteMissing, ReleaseURL: n.client.notesURL("")}
	tag, err := noteTag(version)
	if err != nil {
		return result
	}
	result.Tag = tag
	result.Lookup = NoteUnchecked
	result.ReleaseURL = n.client.notesURL(tag)

	n.mu.Lock()
	defer n.mu.Unlock()
	if entry, ok := n.session[tag]; ok {
		note := entry.note
		result.Note = &note
		result.Saved = entry.saved
		return result
	}
	note, ok := n.readRecordLocked(tag)
	if !ok {
		return result
	}
	n.session[tag] = sessionReleaseNote{note: note, saved: true}
	result.Note = &note
	result.Saved = true
	return result
}

func (n *ReleaseNotes) Refresh(ctx context.Context, version string) ReleaseNotesResult {
	result := ReleaseNotesResult{Version: strings.TrimSpace(version), Lookup: NoteMissing, ReleaseURL: n.client.notesURL("")}
	tag, err := noteTag(version)
	if err != nil {
		return result
	}
	result.Tag = tag
	result.ReleaseURL = n.client.notesURL(tag)

	n.mu.Lock()
	if latest, ok := n.results[tag]; ok && latest.Lookup == NoteFailed && latest.RetryAt != nil && time.Now().Before(*latest.RetryAt) {
		latest.Version = result.Version
		n.mu.Unlock()
		return latest
	}
	n.generations[tag]++
	generation := n.generations[tag]
	n.mu.Unlock()

	note, err := n.client.GetReleaseNotes(ctx, tag)
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.generations[tag] != generation {
		if latest, ok := n.results[tag]; ok {
			latest.Version = result.Version
			return latest
		}
		result.Lookup = NoteFailed
		return n.readResultLocked(result)
	}
	if errors.Is(err, ErrReleaseNotesMissing) {
		result.ReleaseURL = n.client.notesURL("")
		result = n.readResultLocked(result)
		n.results[tag] = result
		return result
	}
	if err != nil {
		result.Lookup = NoteFailed
		var fetchError *ReleaseNotesFetchError
		if errors.As(err, &fetchError) {
			result.RetryAt = fetchError.RetryAt
		}
		result = n.readResultLocked(result)
		n.results[tag] = result
		return result
	}
	result.Lookup = NotePublished

	result.Note = note
	result.Saved = n.writeRecordLocked(*note, tag)
	n.session[tag] = sessionReleaseNote{note: *note, saved: result.Saved}
	n.results[tag] = result
	return result
}

func (n *ReleaseNotes) RememberOffer(info UpdateInfo) ReleaseNotesResult {
	result := ReleaseNotesResult{Version: strings.TrimSpace(info.Version), Lookup: NoteFailed, ReleaseURL: n.client.notesURL("")}
	tag, err := noteTag(info.Version)
	if err != nil {
		return result
	}
	result.Tag = tag
	result.ReleaseURL = n.client.notesURL(tag)
	var publishedAt *time.Time
	if !info.ReleaseDate.IsZero() {
		value := info.ReleaseDate
		publishedAt = &value
	}
	note := ReleaseNote{Tag: tag, Name: info.ReleaseName, Body: info.ReleaseBody, PublishedAt: publishedAt, ReleaseURL: info.ReleaseURL, FetchedAt: time.Now().UTC()}
	if err := n.validateNote(note, tag); err != nil {
		return n.resultWithSavedFallback(result)
	}
	result.Lookup = NotePublished
	result.Note = &note

	n.mu.Lock()
	defer n.mu.Unlock()
	n.lastOfferedTag = tag
	n.generations[tag]++
	result.Saved = n.writeRecordLocked(note, tag)
	n.session[tag] = sessionReleaseNote{note: note, saved: result.Saved}
	n.results[tag] = result
	return result
}

func (n *ReleaseNotes) resultWithSavedFallback(result ReleaseNotesResult) ReleaseNotesResult {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.readResultLocked(result)
}

func (n *ReleaseNotes) readResultLocked(result ReleaseNotesResult) ReleaseNotesResult {
	entry, ok := n.session[result.Tag]
	if !ok {
		note, found := n.readRecordLocked(result.Tag)
		if !found {
			return result
		}
		entry = sessionReleaseNote{note: note, saved: true}
		n.session[result.Tag] = entry
	}
	note := entry.note
	result.Note = &note
	result.Saved = entry.saved
	return result
}
