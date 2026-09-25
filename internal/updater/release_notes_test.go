package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func releaseNotesTestClient(t *testing.T, handler http.HandlerFunc) (*ReleaseClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := NewReleaseClient("owner", "repo")
	client.baseURL = server.URL
	return client, server.Close
}

func writeReleaseResponse(t *testing.T, w http.ResponseWriter, tag, body string) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"tag_name": tag, "name": "Release " + tag, "body": body, "draft": false,
		"html_url":     "https://github.com/owner/repo/releases/tag/" + tag,
		"published_at": "2026-09-05T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseNotes_RestartUsesSavedCopyWhenRefreshFails(t *testing.T) {
	var fail atomic.Bool
	client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeReleaseResponse(t, w, "v1.2.3", "# Saved body")
	})
	defer closeServer()
	dir := t.TempDir()
	first := NewReleaseNotes(client, dir, "1.2.3")

	fresh := first.Refresh(context.Background(), "1.2.3")
	if fresh.Note == nil || fresh.Note.Body != "# Saved body" || !fresh.Saved || fresh.Lookup != NotePublished {
		t.Fatalf("fresh note was not saved: %+v", fresh)
	}

	fail.Store(true)
	restarted := NewReleaseNotes(client, dir, "1.2.3")
	cached := restarted.Read("1.2.3")
	if cached.Note == nil || cached.Note.Body != "# Saved body" || !cached.Saved || cached.Lookup != NoteUnchecked {
		t.Fatalf("restart did not load saved note: %+v", cached)
	}
	failed := restarted.Refresh(context.Background(), "1.2.3")
	if failed.Note == nil || failed.Note.Body != "# Saved body" || !failed.Saved || failed.Lookup != NoteFailed {
		t.Fatalf("failed refresh did not retain saved note: %+v", failed)
	}
}

func TestReleaseNotes_RememberOfferSurvivesRestart(t *testing.T) {
	client := NewReleaseClient("owner", "repo")
	dir := t.TempDir()
	service := NewReleaseNotes(client, dir, "1.0.0")
	published := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)
	result := service.RememberOffer(UpdateInfo{Version: "1.2.3", ReleaseURL: "https://github.com/owner/repo/releases/tag/v1.2.3", ReleaseDate: published, ReleaseName: "Offer", ReleaseBody: "# Offered body"})
	if result.Note == nil || !result.Saved {
		t.Fatalf("offer was not saved: %+v", result)
	}

	restarted := NewReleaseNotes(client, dir, "1.0.0")
	cached := restarted.Read("1.2.3")
	if cached.Note == nil || cached.Note.Body != "# Offered body" || !cached.Saved {
		t.Fatalf("offered note was unavailable after restart: %+v", cached)
	}
}

func TestReleaseNotes_SlowerSameTagRefreshCannotOverwriteNewer(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var requests atomic.Int32
	client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			writeReleaseResponse(t, w, "v1.2.3", "old")
			return
		}
		writeReleaseResponse(t, w, "v1.2.3", "new")
	})
	defer closeServer()
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
	done := make(chan ReleaseNotesResult, 1)
	go func() { done <- service.Refresh(context.Background(), "1.2.3") }()
	<-firstStarted
	newer := service.Refresh(context.Background(), "1.2.3")
	close(releaseFirst)
	<-done

	if newer.Note == nil || newer.Note.Body != "new" || !newer.Saved {
		t.Fatalf("newer refresh failed: %+v", newer)
	}
	read := NewReleaseNotes(client, service.dataDir, "1.2.3").Read("1.2.3")
	if read.Note == nil || read.Note.Body != "new" {
		t.Fatalf("older completion overwrote newer note: %+v", read)
	}
}
