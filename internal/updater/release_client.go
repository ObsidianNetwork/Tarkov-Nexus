package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/mod/semver"
)

const (
	// GitHub repository for releases
	GitHubOwner = "ObsidianNetwork"
	GitHubRepo  = "Tarkov-Nexus"

	defaultAPIBaseURL  = "https://api.github.com"
	releasesListPath   = "/repos/%s/%s/releases?per_page=100"
	requestTimeout     = 30 * time.Second
	updaterAssetSuffix = ".zip"
)

// ghRelease is the subset of the GitHub release payload the updater needs.
type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

// ghAsset is the subset of the GitHub asset payload the updater needs.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// ReleaseClient lists GitHub releases and picks the right one for a channel.
type ReleaseClient struct {
	owner      string
	repo       string
	baseURL    string
	httpClient *http.Client
}

// NewReleaseClient creates a release client for the given owner/repo.
func NewReleaseClient(owner, repo string) *ReleaseClient {
	return &ReleaseClient{
		owner:      owner,
		repo:       repo,
		baseURL:    defaultAPIBaseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// ErrNoReleaseFound is returned when no release matches the channel rules.
var ErrNoReleaseFound = fmt.Errorf("no matching release found")

// List fetches all published releases, newest-first as returned by GitHub.
func (c *ReleaseClient) List(ctx context.Context) ([]ghRelease, error) {
	url := c.baseURL + fmt.Sprintf(releasesListPath, c.owner, c.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch releases: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	return releases, nil
}

// Latest picks the newest release for the channel:
//   - stable: drafts and prereleases excluded
//   - beta: drafts excluded, prereleases included
//
// Ordering is semver-based, so "3.5.0-beta.1" beats "3.4.0" on the beta
// channel while the stable channel ignores it entirely.
func (c *ReleaseClient) Latest(ctx context.Context, channel UpdateChannel) (*ghRelease, error) {
	releases, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return latestFromList(releases, channel)
}

// Get returns the release whose tag exactly matches version ("v" optional).
func (c *ReleaseClient) Get(ctx context.Context, version string) (*ghRelease, error) {
	releases, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return getFromList(releases, version)
}

// latestFromList applies the channel rules to an in-memory list (testable).
func latestFromList(releases []ghRelease, channel UpdateChannel) (*ghRelease, error) {
	var best *ghRelease
	var bestVersion string
	for i := range releases {
		rel := &releases[i]
		if rel.Draft {
			continue
		}
		if channel == ChannelStable && rel.Prerelease {
			continue
		}
		v, err := normalizeSemver(rel.TagName)
		if err != nil {
			continue // malformed tags never break the whole check
		}
		if best == nil {
			best, bestVersion = rel, v
			continue
		}
		cmp := semver.Compare(bestVersion, v)
		// On equal versions prefer the non-prerelease (stable wins over beta).
		if cmp < 0 || (cmp == 0 && best.Prerelease && !rel.Prerelease) {
			best, bestVersion = rel, v
		}
	}
	if best == nil {
		return nil, ErrNoReleaseFound
	}
	return best, nil
}

// getFromList finds an exact-tag match in an in-memory list (testable).
func getFromList(releases []ghRelease, version string) (*ghRelease, error) {
	want, err := normalizeSemver(version)
	if err != nil {
		return nil, fmt.Errorf("invalid version %q: %w", version, err)
	}
	for i := range releases {
		rel := &releases[i]
		if rel.Draft {
			continue
		}
		have, err := normalizeSemver(rel.TagName)
		if err != nil {
			continue
		}
		if have == want {
			return rel, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoReleaseFound, version)
}

// pickUpdaterAsset returns the asset matching the updater naming convention
// (e.g. "Tarkov-Nexus_windows_amd64.zip") for the current platform.
func pickUpdaterAsset(rel *ghRelease) (*ghAsset, error) {
	want := GetAssetName() + updaterAssetSuffix
	for i := range rel.Assets {
		if rel.Assets[i].Name == want {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("release %s has no asset %q", rel.TagName, want)
}
