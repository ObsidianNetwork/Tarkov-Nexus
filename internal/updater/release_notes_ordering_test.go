package updater

import (
	"context"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestReleaseNotes_SupersededRefreshReturnsNewestWholeResult(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		older, newer int
	}{
		{"success after missing", 200, 404}, {"success after failure", 200, 503},
		{"missing after success", 404, 200}, {"failure after success", 503, 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var requests atomic.Int32
			client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				code := scenario.newer
				body := "new body"
				if requests.Add(1) == 1 {
					close(started)
					<-release
					code = scenario.older
					body = "stale body"
				}
				if code != 200 {
					w.WriteHeader(code)
					return
				}
				writeReleaseResponse(t, w, "v1.2.3", body)
			})
			defer closeServer()
			service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
			done := make(chan ReleaseNotesResult, 1)
			go func() { done <- service.Refresh(context.Background(), "1.2.3") }()
			<-started
			latest := service.Refresh(context.Background(), "1.2.3")
			close(release)
			stale := <-done
			if !reflect.DeepEqual(stale, latest) {
				t.Fatalf("superseded result = %+v; latest = %+v", stale, latest)
			}
			saved := NewReleaseNotes(client, service.dataDir, "1.2.3").Read("1.2.3")
			if saved.Note != nil && saved.Note.Body == "stale body" {
				t.Fatal("superseded body reached disk")
			}
		})
	}
}

func TestReleaseNotes_RememberOfferSupersedesPendingFailure(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer closeServer()
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
	done := make(chan ReleaseNotesResult, 1)
	go func() { done <- service.Refresh(context.Background(), "v1.2.3") }()
	<-started
	latest := service.RememberOffer(UpdateInfo{Version: "1.2.3", ReleaseURL: "https://github.com/owner/repo/releases/tag/v1.2.3", ReleaseBody: "offered body"})
	close(release)
	stale := <-done
	latest.Version = "v1.2.3"
	if !reflect.DeepEqual(stale, latest) {
		t.Fatalf("pending failure changed offered result: %+v, want %+v", stale, latest)
	}
}

func TestReleaseNotes_SupersededRefreshWhileNewerPending(t *testing.T) {
	started, newerStarted := make(chan struct{}), make(chan struct{})
	release, releaseNewer := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-release
			writeReleaseResponse(t, w, "v1.2.3", "old")
			return
		}
		close(newerStarted)
		<-releaseNewer
		writeReleaseResponse(t, w, "v1.2.3", "new")
	})
	defer closeServer()
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
	first, second := make(chan ReleaseNotesResult, 1), make(chan ReleaseNotesResult, 1)
	go func() { first <- service.Refresh(context.Background(), "1.2.3") }()
	<-started
	go func() { second <- service.Refresh(context.Background(), "1.2.3") }()
	<-newerStarted
	close(release)
	stale := <-first
	// Release the handler before asserting so a failure cannot hang server cleanup.
	close(releaseNewer)
	latest := <-second
	if stale.Lookup != NoteFailed || stale.Note != nil {
		t.Fatalf("superseded result falsely reported completed content: %+v", stale)
	}
	if latest.Lookup != NotePublished || latest.Note == nil || latest.Note.Body != "new" {
		t.Fatalf("newer result lost: %+v", latest)
	}
}
