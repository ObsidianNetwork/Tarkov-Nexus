package updater

import "time"

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Version     string    `json:"version"`
	ReleaseURL  string    `json:"releaseUrl"`
	ReleaseDate time.Time `json:"releaseDate"`
	ReleaseName string    `json:"releaseName"`
	ReleaseBody string    `json:"releaseBody"`
	AssetURL    string    `json:"assetUrl"`
	AssetName   string    `json:"assetName"`
	AssetSize   int64     `json:"assetSize"`
	IsPrerelease bool     `json:"isPrerelease"`
}

// UpdateStatus represents the current update process status
type UpdateStatus struct {
	Checking         bool   `json:"checking"`
	Downloading      bool   `json:"downloading"`
	Installing       bool   `json:"installing"`
	UpdateAvailable  bool   `json:"updateAvailable"`
	CurrentVersion   string `json:"currentVersion"`
	LatestVersion    string `json:"latestVersion"`
	DownloadProgress int    `json:"downloadProgress"` // 0-100
	Error            string `json:"error"`
	LastChecked      string `json:"lastChecked"`
}

// UpdateChannel represents the update channel (stable or beta)
type UpdateChannel string

const (
	ChannelStable UpdateChannel = "stable"
	ChannelBeta   UpdateChannel = "beta"
)
