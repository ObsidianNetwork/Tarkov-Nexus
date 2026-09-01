package updater

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/inconshreveable/go-update"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"tarkov-screenshot-analyzer/internal/logger"
)

const (
	// GitHub repository for releases
	GitHubOwner = "ObsidianNetwork"
	GitHubRepo  = "Tarkov-Nexus"
)

// Updater manages application updates
type Updater struct {
	logger        logger.Logger
	status        UpdateStatus
	statusMutex   sync.RWMutex
	channel       UpdateChannel
	eventHandlers map[string][]func(interface{})
	handlerMutex  sync.RWMutex
}

// NewUpdater creates a new updater instance
func NewUpdater(log logger.Logger, channel UpdateChannel) *Updater {
	return &Updater{
		logger:        log,
		channel:       channel,
		eventHandlers: make(map[string][]func(interface{})),
		status: UpdateStatus{
			CurrentVersion: Version,
		},
	}
}

// CheckForUpdates checks if a new version is available
func (u *Updater) CheckForUpdates() (*UpdateInfo, error) {
	u.setStatus(func(s *UpdateStatus) {
		s.Checking = true
		s.Error = ""
	})
	defer u.setStatus(func(s *UpdateStatus) {
		s.Checking = false
		s.LastChecked = time.Now().Format(time.RFC3339)
	})

	u.logger.Info("Checking for updates...")

	// Get latest release
	latest, found, err := selfupdate.DetectLatest(GitHubOwner + "/" + GitHubRepo)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to check for updates: %v", err))
		u.setError(fmt.Sprintf("Update check failed: %v", err))
		return nil, err
	}

	if !found {
		u.logger.Info("No releases found")
		return nil, fmt.Errorf("no releases found")
	}

	// Check if update is available
	currentVersion := "v" + Version
	isNewer, err := IsNewerVersion(Version, latest.Version.String())
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to compare versions: %v", err))
		return nil, err
	}

	if !isNewer {
		u.logger.Info(fmt.Sprintf("Already on latest version: %s", currentVersion))
		u.setStatus(func(s *UpdateStatus) {
			s.UpdateAvailable = false
			s.LatestVersion = latest.Version.String()
		})
		return nil, nil
	}

	u.logger.Info(fmt.Sprintf("Update available: %s -> %s", currentVersion, latest.Version))

	// Build UpdateInfo
	publishedAt := time.Time{}
	if latest.PublishedAt != nil {
		publishedAt = *latest.PublishedAt
	}

	info := &UpdateInfo{
		Version:      latest.Version.String(),
		ReleaseURL:   latest.URL,
		ReleaseDate:  publishedAt,
		ReleaseName:  latest.Name,
		ReleaseBody:  latest.ReleaseNotes,
		AssetURL:     latest.AssetURL,
		AssetName:    "",
		AssetSize:    int64(latest.AssetByteSize),
		IsPrerelease: false, // Library doesn't expose this
	}

	u.setStatus(func(s *UpdateStatus) {
		s.UpdateAvailable = true
		s.LatestVersion = latest.Version.String()
	})

	u.emit("update:available", info)

	return info, nil
}

// DownloadAndInstall downloads and installs the update
func (u *Updater) DownloadAndInstall(version string) error {
	u.setStatus(func(s *UpdateStatus) {
		s.Downloading = true
		s.Error = ""
	})

	u.logger.Info(fmt.Sprintf("Downloading update: %s", version))
	u.emit("update:downloading", map[string]interface{}{"version": version})

	// Get latest release
	latest, found, err := selfupdate.DetectLatest(GitHubOwner + "/" + GitHubRepo)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to detect latest version: %v", err))
		u.setError(fmt.Sprintf("Version detection failed: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Downloading = false
		})
		return err
	}

	if !found {
		err := fmt.Errorf("release not found")
		u.logger.Error(err.Error())
		u.setError(err.Error())
		u.setStatus(func(s *UpdateStatus) {
			s.Downloading = false
		})
		return err
	}

	u.setStatus(func(s *UpdateStatus) {
		s.Downloading = false
		s.Installing = true
	})

	u.logger.Info("Installing update...")
	u.emit("update:installing", map[string]interface{}{"version": version})

	// Download the zip file
	u.logger.Info(fmt.Sprintf("Downloading from: %s", latest.AssetURL))
	resp, err := http.Get(latest.AssetURL)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to download update: %v", err))
		u.setError(fmt.Sprintf("Download failed: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("download failed with status: %d", resp.StatusCode)
		u.logger.Error(err.Error())
		u.setError(err.Error())
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}

	// Save to temporary file
	tmpZip, err := os.CreateTemp("", "update-*.zip")
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to create temp file: %v", err))
		u.setError(fmt.Sprintf("Failed to create temp file: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}
	defer os.Remove(tmpZip.Name())

	_, err = io.Copy(tmpZip, resp.Body)
	tmpZip.Close()
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to save download: %v", err))
		u.setError(fmt.Sprintf("Failed to save download: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}

	// Extract the first executable from the zip
	u.logger.Info("Extracting executable from archive...")
	binary, err := extractFirstExeFromZip(tmpZip.Name())
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to extract executable: %v", err))
		u.setError(fmt.Sprintf("Extraction failed: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}
	defer binary.Close()

	// Apply the update - this works regardless of executable name
	// go-update handles Windows file locking properly
	u.logger.Info("Applying update to executable...")
	err = update.Apply(binary, update.Options{})
	if err != nil {
		// Attempt rollback if update fails
		if rerr := update.RollbackError(err); rerr != nil {
			u.logger.Error(fmt.Sprintf("Rollback failed: %v", rerr))
		}
		u.logger.Error(fmt.Sprintf("Failed to apply update: %v", err))
		u.setError(fmt.Sprintf("Update failed: %v", err))
		u.setStatus(func(s *UpdateStatus) {
			s.Installing = false
		})
		return err
	}

	// Update successful - works with any executable name
	u.logger.Info("Update installed successfully. Application will restart.")
	u.setStatus(func(s *UpdateStatus) {
		s.Installing = false
		s.UpdateAvailable = false
	})

	u.emit("update:ready", map[string]interface{}{
		"version":        version,
		"needsRestart":   true,
		"restartMessage": "The application will now restart to complete the update.",
	})

	return nil
}

// extractFirstExeFromZip extracts the first .exe file from a zip archive
// This allows updates to work regardless of executable name
func extractFirstExeFromZip(zipPath string) (io.ReadCloser, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	// Find first .exe file in the archive
	for _, f := range r.File {
		// On Windows, look for .exe files
		// On other platforms, look for files without extension (Unix executables)
		isExecutable := false
		if runtime.GOOS == "windows" {
			isExecutable = filepath.Ext(f.Name) == ".exe"
		} else {
			// On Unix, executables typically have no extension
			isExecutable = filepath.Ext(f.Name) == ""
		}

		if isExecutable {
			rc, err := f.Open()
			if err != nil {
				r.Close()
				return nil, fmt.Errorf("failed to open file in zip: %w", err)
			}

			// Create a temp file to hold the extracted binary
			tmpFile, err := os.CreateTemp("", "update-exe-*")
			if err != nil {
				rc.Close()
				r.Close()
				return nil, fmt.Errorf("failed to create temp file: %w", err)
			}

			// Copy the executable to temp file
			_, err = io.Copy(tmpFile, rc)
			rc.Close()
			r.Close()

			if err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("failed to copy executable: %w", err)
			}

			// Seek back to start for reading
			tmpFile.Seek(0, 0)
			return tmpFile, nil
		}
	}

	r.Close()
	return nil, fmt.Errorf("no executable found in archive")
}

// RestartApplication restarts the application to apply the update
func (u *Updater) RestartApplication() error {
	u.logger.Info("Restarting application to apply update...")

	// Get the current executable path
	exe, err := os.Executable()
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to get executable path: %v", err))
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks if any
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to resolve executable path: %v", err))
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// On Windows, create a batch script to restart after a delay
	if runtime.GOOS == "windows" {
		return u.restartWindows(exe)
	}

	// On Unix-like systems, use exec to replace the current process
	return u.restartUnix(exe)
}

// restartWindows creates a batch script to restart the application on Windows
func (u *Updater) restartWindows(exePath string) error {
	// Create a temporary batch script
	batchContent := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > nul
start "" "%s"
del "%%~f0"`, exePath)

	// Get temp directory
	tempDir := os.TempDir()
	batchPath := filepath.Join(tempDir, "tarkov_nexus_restart.bat")

	// Write batch script
	if err := os.WriteFile(batchPath, []byte(batchContent), 0755); err != nil {
		u.logger.Error(fmt.Sprintf("Failed to create restart script: %v", err))
		return fmt.Errorf("failed to create restart script: %w", err)
	}

	u.logger.Info(fmt.Sprintf("Created restart script: %s", batchPath))

	// Execute the batch script in a detached process
	cmd := exec.Command("cmd", "/C", "start", "/B", batchPath)
	if err := cmd.Start(); err != nil {
		u.logger.Error(fmt.Sprintf("Failed to execute restart script: %v", err))
		return fmt.Errorf("failed to execute restart script: %w", err)
	}

	u.logger.Info("Restart script launched successfully")
	return nil
}

// restartUnix restarts the application on Unix-like systems
func (u *Updater) restartUnix(exePath string) error {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = filepath.Dir(exePath)
	}

	// Get command line arguments
	args := os.Args[1:]

	// Fork a new process
	cmd := exec.Command(exePath, args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		u.logger.Error(fmt.Sprintf("Failed to start new process: %v", err))
		return fmt.Errorf("failed to start new process: %w", err)
	}

	u.logger.Info("New process started successfully")
	return nil
}

// GetStatus returns the current update status
func (u *Updater) GetStatus() UpdateStatus {
	u.statusMutex.RLock()
	defer u.statusMutex.RUnlock()
	return u.status
}

// SetChannel sets the update channel (stable or beta)
func (u *Updater) SetChannel(channel UpdateChannel) {
	u.statusMutex.Lock()
	defer u.statusMutex.Unlock()
	u.channel = channel
	u.logger.Info(fmt.Sprintf("Update channel set to: %s", channel))
}

// GetChannel returns the current update channel
func (u *Updater) GetChannel() UpdateChannel {
	u.statusMutex.RLock()
	defer u.statusMutex.RUnlock()
	return u.channel
}

// setStatus updates the status using a modifier function
func (u *Updater) setStatus(modifier func(*UpdateStatus)) {
	u.statusMutex.Lock()
	defer u.statusMutex.Unlock()
	modifier(&u.status)
}

// setError sets the error message in status
func (u *Updater) setError(msg string) {
	u.statusMutex.Lock()
	defer u.statusMutex.Unlock()
	u.status.Error = msg
}

// OnEvent registers an event handler
func (u *Updater) OnEvent(event string, handler func(interface{})) {
	u.handlerMutex.Lock()
	defer u.handlerMutex.Unlock()
	u.eventHandlers[event] = append(u.eventHandlers[event], handler)
}

// emit triggers event handlers
func (u *Updater) emit(event string, data interface{}) {
	u.handlerMutex.RLock()
	defer u.handlerMutex.RUnlock()

	if handlers, exists := u.eventHandlers[event]; exists {
		for _, handler := range handlers {
			go func(h func(interface{}), ev string) {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("Panic in updater event handler [%s]: %v\n", ev, r)
					}
				}()
				h(data)
			}(handler, event)
		}
	}
}

// GetAssetName returns the expected asset name for the current platform
func GetAssetName() string {
	platform := runtime.GOOS
	arch := runtime.GOARCH

	// Map platform/arch to expected asset names
	switch platform {
	case "windows":
		return fmt.Sprintf("TarkovMapSync-windows-%s.exe", arch)
	case "darwin":
		return fmt.Sprintf("TarkovMapSync-darwin-%s", arch)
	case "linux":
		return fmt.Sprintf("TarkovMapSync-linux-%s", arch)
	default:
		return fmt.Sprintf("TarkovMapSync-%s-%s", platform, arch)
	}
}
