package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const threePartFleetNote = "tags are three-part semver; malformed tags must be skipped, never fatal"

func testReleases() []ghRelease {
	return []ghRelease{
		{TagName: "v3.4.0-beta.2", Prerelease: true, PublishedAt: time.Now().Add(-1 * time.Hour),
			Assets: []ghAsset{{Name: "Tarkov-Nexus_windows_amd64.zip", BrowserDownloadURL: "https://example.com/b2.zip"}}},
		{TagName: "v3.4.0-beta.1", Prerelease: true, PublishedAt: time.Now().Add(-48 * time.Hour),
			Assets: []ghAsset{{Name: "Tarkov-Nexus_windows_amd64.zip", BrowserDownloadURL: "https://example.com/b1.zip"}}},
		{TagName: "v3.3.3", PublishedAt: time.Now().Add(-720 * time.Hour),
			Assets: []ghAsset{{Name: "Tarkov-Nexus_windows_amd64.zip", BrowserDownloadURL: "https://example.com/s.zip"}}},
		{TagName: "bad-tag", PublishedAt: time.Now(), Assets: []ghAsset{{Name: "x.zip"}}},
	}
}

func TestLatestFromList_StableSkipsPrereleasesAndDrafts(t *testing.T) {
	releases := append(testReleases(), ghRelease{TagName: "v9.9.9", Draft: true})
	rel, err := latestFromList(releases, ChannelStable)
	if err != nil {
		t.Fatalf("latestFromList error: %v", err)
	}
	if rel.TagName != "v3.3.3" {
		t.Errorf("stable channel picked %q, want v3.3.3 (drafts and prereleases ignored — %s)", rel.TagName, threePartFleetNote)
	}
}

func TestLatestFromList_BetaIncludesPrereleases(t *testing.T) {
	rel, err := latestFromList(testReleases(), ChannelBeta)
	if err != nil {
		t.Fatalf("latestFromList error: %v", err)
	}
	if rel.TagName != "v3.4.0-beta.2" {
		t.Errorf("beta channel picked %q, want v3.4.0-beta.2", rel.TagName)
	}
}

func TestLatestFromList_BetaPrefersStableOverEqualBeta(t *testing.T) {
	// A stable release and a same-version beta both present: stable wins.
	releases := []ghRelease{
		{TagName: "v3.4.0", Prerelease: false},
		{TagName: "v3.4.0-beta.9", Prerelease: true},
	}
	rel, err := latestFromList(releases, ChannelBeta)
	if err != nil {
		t.Fatalf("latestFromList error: %v", err)
	}
	if rel.TagName != "v3.4.0" {
		t.Errorf("beta channel picked %q, want stable v3.4.0 on tie", rel.TagName)
	}
}

func TestLatestFromList_NoneMatchChannel(t *testing.T) {
	onlyBeta := []ghRelease{{TagName: "v3.4.0-beta.1", Prerelease: true}}
	if _, err := latestFromList(onlyBeta, ChannelStable); !errors.Is(err, ErrNoReleaseFound) {
		t.Errorf("expected ErrNoReleaseFound, got %v", err)
	}
}

func TestGetFromList_ExactTag(t *testing.T) {
	releases := testReleases()
	rel, err := getFromList(releases, "3.4.0-beta.1") // no v prefix
	if err != nil {
		t.Fatalf("getFromList error: %v", err)
	}
	if rel.TagName != "v3.4.0-beta.1" {
		t.Errorf("Get matched %q, want v3.4.0-beta.1", rel.TagName)
	}
	if _, err := getFromList(releases, "1.2.3"); !errors.Is(err, ErrNoReleaseFound) {
		t.Errorf("expected ErrNoReleaseFound for unknown version, got %v", err)
	}
	if _, err := getFromList(releases, "garbage!!"); err == nil || errors.Is(err, ErrNoReleaseFound) {
		t.Errorf("invalid version should be a parse error, got %v", err)
	}
}

func TestPickUpdaterAsset(t *testing.T) {
	// The updater-named asset is platform-specific; build the expected name
	// from the same source of truth the picker uses (CI emits e.g.
	// "Tarkov-Nexus_windows_amd64.zip" on the release workflow).
	wantName := GetAssetName() + ".zip"
	rel := &ghRelease{
		TagName: "v3.4.0",
		Assets: []ghAsset{
			{Name: "TarkovNexus-v3.4.0-Windows-x64.zip"}, // user-facing zip must not be picked
			{Name: wantName, BrowserDownloadURL: "https://example.com/updater.zip", Size: 123},
		},
	}
	asset, err := pickUpdaterAsset(rel)
	if err != nil {
		t.Fatalf("pickUpdaterAsset error: %v", err)
	}
	if asset.Name != wantName {
		t.Errorf("picked %q, want %q", asset.Name, wantName)
	}
	if _, err := pickUpdaterAsset(&ghRelease{TagName: "v1.0.0"}); err == nil {
		t.Error("missing asset should error")
	}
}

func TestReleaseClient_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/ObsidianNetwork/Tarkov-Nexus/releases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testReleases())
	}))
	defer srv.Close()

	c := NewReleaseClient(GitHubOwner, GitHubRepo)
	c.baseURL = srv.URL

	releases, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(releases) != 4 {
		t.Errorf("List returned %d releases, want 4", len(releases))
	}

	rel, err := c.Latest(context.Background(), ChannelBeta)
	if err != nil {
		t.Fatalf("Latest error: %v", err)
	}
	if rel.TagName != "v3.4.0-beta.2" {
		t.Errorf("Latest(beta) = %q, want v3.4.0-beta.2", rel.TagName)
	}
}

func TestReleaseClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewReleaseClient(GitHubOwner, GitHubRepo)
	c.baseURL = srv.URL

	if _, err := c.List(context.Background()); err == nil {
		t.Error("HTTP 403 should surface as an error, not empty results")
	}
}

func TestUpdater_ListBetaVersions(t *testing.T) {
	u := NewUpdater(consoleTestLogger{}, ChannelBeta)
	releases := []ghRelease{
		{TagName: "v3.4.0-beta.1", Prerelease: true, PublishedAt: time.Now().Add(-48 * time.Hour)},
		{TagName: "v3.4.0-beta.2", Prerelease: true, PublishedAt: time.Now().Add(-1 * time.Hour)},
		{TagName: "v3.3.3", Prerelease: false},
		{TagName: "v9.9.9-rc.1", Draft: true, Prerelease: true},
		{TagName: "garbage-tag", Prerelease: true},
	}
	u.releaseClient = &ReleaseClient{
		owner: "o", repo: "r", baseURL: "unused",
		httpClient: &http.Client{Transport: listStubTransport(releases)},
	}

	opts, err := u.ListBetaVersions(context.Background())
	if err != nil {
		t.Fatalf("ListBetaVersions error: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("got %d options, want 2 (stable, drafts and malformed tags excluded)", len(opts))
	}
	if opts[0].Version != "3.4.0-beta.2" || !opts[0].IsLatest {
		t.Errorf("first option = %+v, want 3.4.0-beta.2 marked latest", opts[0])
	}
	if opts[1].Version != "3.4.0-beta.1" || opts[1].IsLatest {
		t.Errorf("second option = %+v, want 3.4.0-beta.1 not latest", opts[1])
	}
}

// listStubTransport serves a canned release list for any request.
func listStubTransport(releases []ghRelease) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(releases)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
