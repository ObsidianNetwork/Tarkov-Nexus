package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type notesOutcomeFixture struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	Saved   ReleaseNotesResult `json:"saved"`
	Result  ReleaseNotesResult `json:"result"`
}

func TestReleaseNotes_HTTPOutcomesPreserveIdentityAndSavedContent(t *testing.T) {
	valid := `{"tag_name":"v1.2.3","draft":false,"body":"# Published notes","html_url":"https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/tag/v1.2.3","published_at":"2026-09-05T00:00:00Z"}`
	cases := []struct {
		name    string
		status  int
		body    string
		lookup  NoteLookup
		timeout bool
	}{
		{"content", 200, valid, NotePublished, false},
		{"empty", 200, strings.Replace(valid, `"# Published notes"`, `""`, 1), NotePublished, false},
		{"whitespace", 200, strings.Replace(valid, `"# Published notes"`, `"  \n"`, 1), NotePublished, false},
		{"null", 200, strings.Replace(valid, `"# Published notes"`, `null`, 1), NotePublished, false},
		{"missing", 404, `{}`, NoteMissing, false},
		{"forbidden", 403, `{}`, NoteFailed, false},
		{"rate-limit", 429, `{}`, NoteFailed, false},
		{"server-error", 500, `{}`, NoteFailed, false},
		{"malformed", 200, `{`, NoteFailed, false},
		{"mismatched", 200, strings.Replace(valid, `"tag_name":"v1.2.3"`, `"tag_name":"v2.0.0"`, 1), NoteFailed, false},
		{"oversized-body", 200, strings.Replace(valid, `# Published notes`, strings.Repeat("x", maxNoteBodyBytes+1), 1), NoteFailed, false},
		{"oversized-response", 200, strings.Repeat(" ", maxNoteResponseBytes+1), NoteFailed, false},
		{"timeout", 200, valid, NoteFailed, true},
	}
	var fixtures []notesOutcomeFixture
	for _, tc := range cases {
		for _, cached := range []string{"none", "text", "empty"} {
			t.Run(tc.name+"/"+cached, func(t *testing.T) {
				client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					if tc.timeout {
						<-r.Context().Done()
						return
					}
					if tc.status == 403 || tc.status == 429 {
						w.Header().Set("Retry-After", "60")
					}
					w.WriteHeader(tc.status)
					if _, err := w.Write([]byte(tc.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				})
				defer closeServer()
				client.owner, client.repo = "ObsidianNetwork", "Tarkov-Nexus"
				service := NewReleaseNotes(client, t.TempDir(), "1.2.3")
				cachedBody := "# Saved notes\n\nPreviously fetched release text."
				if cached == "empty" {
					cachedBody = " \n"
				}
				if cached != "none" {
					seed := service.RememberOffer(UpdateInfo{Version: "1.2.3", ReleaseURL: client.notesURL("v1.2.3"), ReleaseBody: cachedBody, ReleaseDate: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)})
					if !seed.Saved {
						t.Fatal("could not seed temporary cache")
					}
				}
				saved := service.Read("1.2.3")
				ctx := context.Background()
				if tc.timeout {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
					defer cancel()
				}
				result := service.Refresh(ctx, "1.2.3")
				if result.Version != "1.2.3" || result.Lookup != tc.lookup {
					t.Fatalf("wrong classification or version: %+v", result)
				}
				expectedURL := client.notesURL("v1.2.3")
				if tc.lookup == NoteMissing {
					expectedURL = client.notesURL("")
				}
				if result.ReleaseURL != expectedURL {
					t.Fatalf("wrong outcome link: %s", result.ReleaseURL)
				}
				if tc.lookup != NotePublished {
					if cached == "none" && result.Note != nil {
						t.Fatal("failure manufactured note content")
					}
					if cached != "none" && (result.Note == nil || result.Note.Body != cachedBody || !result.Saved) {
						t.Fatalf("saved content lost: %+v", result)
					}
				} else if result.Note == nil || !result.Saved {
					t.Fatal("published record was not saved")
				}
				if (tc.status == 403 || tc.status == 429) && result.RetryAt == nil {
					t.Fatal("rate-limit retry time lost")
				}
				fixtures = append(fixtures, notesOutcomeFixture{Name: tc.name + "-" + cached, Version: "1.2.3", Saved: saved, Result: result})
			})
		}
	}
	t.Run("development version", func(t *testing.T) {
		client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("development version performed HTTP lookup") })
		defer closeServer()
		client.owner, client.repo = "ObsidianNetwork", "Tarkov-Nexus"
		service := NewReleaseNotes(client, t.TempDir(), "dev-build")
		result := service.Refresh(context.Background(), "dev-build")
		if result.Version != "dev-build" || result.Lookup != NoteMissing || result.Note != nil || result.ReleaseURL != client.notesURL("") {
			t.Fatalf("wrong development outcome: %+v", result)
		}
		fixtures = append(fixtures, notesOutcomeFixture{Name: "development", Version: "dev-build", Saved: service.Read("dev-build"), Result: result})
	})
	if output := os.Getenv("TAR14_QA_FIXTURES"); output != "" {
		data, err := json.MarshalIndent(fixtures, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReleaseNotes_RetryDeadlineSuppressesAnotherRequest(t *testing.T) {
	var requests atomic.Int32
	client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	defer closeServer()
	client.owner, client.repo = "ObsidianNetwork", "Tarkov-Nexus"
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")

	first := service.Refresh(context.Background(), "1.2.3")
	second := service.Refresh(context.Background(), "1.2.3")
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1 before retry deadline", got)
	}
	if first.RetryAt == nil || second.RetryAt == nil || !first.RetryAt.Equal(*second.RetryAt) {
		t.Fatalf("retry deadline was not retained: first=%v second=%v", first.RetryAt, second.RetryAt)
	}
	if second.Lookup != NoteFailed || second.Version != "1.2.3" {
		t.Fatalf("wrong retained failure: %+v", second)
	}
}
