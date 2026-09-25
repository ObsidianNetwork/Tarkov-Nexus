package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseNotes_ExactTagBeyondRecentPage(t *testing.T) {
	tag := "v1.2.3-beta.2+Archive"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/owner/repo/releases/tags/"+tag || r.URL.RawQuery != "" {
			t.Errorf("expected only exact-tag lookup, got %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("missing GitHub Accept header")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": tag, "name": "Archived release", "body": "# Full notes\n\nOlder than the recent page.",
			"draft": false, "html_url": "https://github.com/owner/repo/releases/tag/" + tag,
			"published_at": "2020-01-02T03:04:05Z", "assets": []string{},
		})
	}))
	defer server.Close()
	client := NewReleaseClient("owner", "repo")
	client.baseURL = server.URL

	note, err := client.GetReleaseNotes(context.Background(), tag)

	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || note.Tag != tag || note.Body != "# Full notes\n\nOlder than the recent page." || note.PublishedAt == nil {
		t.Fatalf("lost exact release content: requests=%d note=%+v", requests, note)
	}
}

func TestReleaseNotes_RejectsCrossOriginRedirect(t *testing.T) {
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/private", http.StatusFound)
	}))
	defer origin.Close()
	client := NewReleaseClient("owner", "repo")
	client.baseURL = origin.URL

	if _, err := client.GetReleaseNotes(context.Background(), "v1.2.3"); err == nil {
		t.Fatal("cross-origin redirect was accepted")
	}
	if targetRequests != 0 {
		t.Fatalf("redirect target received %d request(s)", targetRequests)
	}
}

func TestReleaseNotes_StopsSameOriginRedirectLoop(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer server.Close()
	client := NewReleaseClient("owner", "repo")
	client.baseURL = server.URL

	if _, err := client.GetReleaseNotes(context.Background(), "v1.2.3"); err == nil {
		t.Fatal("same-origin redirect loop was accepted")
	}
	if requests != 10 {
		t.Fatalf("redirect policy made %d requests, want 10", requests)
	}
}

func TestReleaseNotes_PreservesExactIdentity(t *testing.T) {
	for _, version := range []string{"1.2.3", "v1.2.3", " 1.2.3 ", "1.2.3-beta.2+Archive"} {
		t.Run(version, func(t *testing.T) {
			tag, err := noteTag(version)
			if err != nil || tag != "v"+strings.TrimPrefix(strings.TrimSpace(version), "v") {
				t.Fatalf("identity changed: %q %v", tag, err)
			}
		})
	}
	for _, version := range []string{"dev", "1", "1.2", "01.2.3", "1.2.3/other", "1.2.3\nother"} {
		t.Run(version, func(t *testing.T) {
			if _, err := noteTag(version); err == nil {
				t.Fatal("accepted a non-exact release identity")
			}
		})
	}
}

func TestReleaseNotes_ValidatesResponse(t *testing.T) {
	valid := `{"tag_name":"v1.2.3","draft":false,"body":"# Notes","html_url":"https://github.com/owner/repo/releases/tag/v1.2.3","published_at":"2020-01-02T03:04:05Z"}`
	cases := []struct {
		name    string
		body    string
		status  int
		valid   bool
		missing bool
	}{
		{"published", valid, 200, true, false},
		{"empty", strings.Replace(valid, `"# Notes"`, `""`, 1), 200, true, false},
		{"null body", strings.Replace(valid, `"# Notes"`, `null`, 1), 200, true, false},
		{"missing body", strings.Replace(valid, `"body":"# Notes",`, ``, 1), 200, false, false},
		{"draft absent", strings.Replace(valid, `"draft":false,`, ``, 1), 200, false, false},
		{"draft", strings.Replace(valid, `false`, `true`, 1), 200, false, false},
		{"wrong tag", strings.Replace(valid, `"tag_name":"v1.2.3"`, `"tag_name":"v2.0.0"`, 1), 200, false, false},
		{"wrong repo", strings.Replace(valid, `/owner/repo/`, `/other/repo/`, 1), 200, false, false},
		{"wrong URL tag", strings.Replace(valid, `/tag/v1.2.3`, `/tag/v2.0.0`, 1), 200, false, false},
		{"unsafe URL", strings.Replace(valid, `https://github.com`, `javascript:`, 1), 200, false, false},
		{"invalid date", strings.Replace(valid, `2020-01-02T03:04:05Z`, `yesterday`, 1), 200, false, false},
		{"wrong body type", strings.Replace(valid, `"# Notes"`, `{}`, 1), 200, false, false},
		{"trailing JSON", valid + `{}`, 200, false, false},
		{"absent", `{}`, 404, false, true},
		{"forbidden", `{}`, 403, false, false},
		{"server error", `{}`, 500, false, false},
		{"oversize body", strings.Replace(valid, `# Notes`, strings.Repeat("x", 1024*1024+1), 1), 200, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewReleaseClient("owner", "repo")
			client.baseURL = server.URL

			note, err := client.GetReleaseNotes(context.Background(), "v1.2.3")

			if (err == nil) != tc.valid || (note != nil) != tc.valid || errors.Is(err, ErrReleaseNotesMissing) != tc.missing {
				t.Fatalf("incorrect outcome: note=%+v err=%v", note, err)
			}
		})
	}
}

func TestReleaseNotes_RefreshKeepsVersionWithoutSaving(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := NewReleaseClient("owner", "repo")
	client.baseURL = server.URL
	service := NewReleaseNotes(client, t.TempDir(), "1.2.3")

	result := service.Refresh(context.Background(), "1.2.3")

	if result.Version != "1.2.3" || result.Tag != "v1.2.3" || result.Lookup != NoteMissing || result.Note != nil || result.Saved {
		t.Fatalf("incorrect missing result: %+v", result)
	}
	if result.ReleaseURL != "https://github.com/owner/repo/releases" {
		t.Fatalf("missing release used an exact link: %s", result.ReleaseURL)
	}
}
