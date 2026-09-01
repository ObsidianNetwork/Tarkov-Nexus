package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
)

// Config represents the application configuration
type Config struct {
	RemoteID                 string           `json:"remoteId"`
	ScreenshotDir            string           `json:"screenshotDir"`
	LogsDir                  string           `json:"logsDir"`
	AutoConnect              bool             `json:"autoConnect"`
	AutoProcessExisting      bool             `json:"autoProcessExisting"`
	PositionUpdateThrottleMs int              `json:"positionUpdateThrottleMs"`
	DebugLogging             bool             `json:"debugLogging"`
	DarkMode                 bool             `json:"darkMode"`
	EnableQuestTracking      bool             `json:"enableQuestTracking"`
	TarkovTrackerURL         string           `json:"tarkovTrackerUrl"`
	TarkovTrackerToken       string           `json:"tarkovTrackerToken"`
	TarkovTrackerGameMode    string           `json:"tarkovTrackerGameMode"`
	MonitorOptions           MonitorOptions   `json:"monitorOptions"`
	ReconnectOptions         ReconnectOptions `json:"reconnectOptions"`
	UpdateSettings           UpdateSettings   `json:"updateSettings"`
	PartySettings            PartySettings    `json:"partySettings"`
	SetupComplete            bool             `json:"setupComplete"`
}

// MonitorOptions configuration for file monitoring
type MonitorOptions struct {
	DebounceTimeMs int  `json:"debounceTimeMs"`
	IgnoreInitial  bool `json:"ignoreInitial"`
}

// ReconnectOptions configuration for WebSocket reconnection
type ReconnectOptions struct {
	ReconnectIntervalMs  int `json:"reconnectIntervalMs"`
	MaxReconnectAttempts int `json:"maxReconnectAttempts"`
}

// UpdateSettings configuration for auto-updates
type UpdateSettings struct {
	EnableAutoCheck bool   `json:"enableAutoCheck"`
	CheckInterval   int    `json:"checkIntervalHours"`
	UpdateChannel   string `json:"updateChannel"`
}

// PartySettings configuration for party/multiplayer features
type PartySettings struct {
	Enabled     bool   `json:"enabled"`
	ServerURL   string `json:"serverUrl"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
	ServerPort  int    `json:"serverPort,omitempty"`
	PartyCode   string `json:"partyCode,omitempty"`
}

// DefaultConfig returns first-run defaults used when no config.json exists
// and as the overlay base for LoadConfig. AutoConnect is false so the
// desktop app does not start integration before the setup wizard finishes.
func DefaultConfig() *Config {
	return &Config{
		AutoConnect:              false,
		AutoProcessExisting:      false,
		PositionUpdateThrottleMs: 1000,
		EnableQuestTracking:      false,
		TarkovTrackerURL:         "https://tarkovtracker.org/api/v2",
		TarkovTrackerGameMode:    "pvp",
		DarkMode:                 true,
		MonitorOptions: MonitorOptions{
			DebounceTimeMs: 500,
			IgnoreInitial:  false,
		},
		ReconnectOptions: ReconnectOptions{
			ReconnectIntervalMs:  5000,
			MaxReconnectAttempts: 10,
		},
		UpdateSettings: UpdateSettings{
			EnableAutoCheck: true,
			CheckInterval:   24,
			UpdateChannel:   "stable",
		},
		PartySettings: PartySettings{
			Enabled:     false,
			ServerURL:   "wss://party.tarkov.nexus/ws",
			DisplayName: "Player",
		},
	}
}

// LoadConfig loads configuration from file, environment variables, and defaults.
// A missing config.json is an error so the desktop app can apply DefaultConfig
// (via createDefaultConfig) instead of treating first launch as a successful load.
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	if configPath == "" {
		configPath = "config.json"
	}

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to access config file: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	fmt.Printf("Loaded configuration from %s\n", configPath)

	if remoteID := os.Getenv("TARKOV_REMOTE_ID"); remoteID != "" {
		config.RemoteID = remoteID
	}
	if screenshotDir := os.Getenv("TARKOV_SCREENSHOT_DIR"); screenshotDir != "" {
		config.ScreenshotDir = screenshotDir
	}
	if trackerURL := os.Getenv("TARKOV_TRACKER_URL"); trackerURL != "" {
		config.TarkovTrackerURL = trackerURL
	}
	if trackerToken := os.Getenv("TARKOV_TRACKER_TOKEN"); trackerToken != "" {
		config.TarkovTrackerToken = trackerToken
	}
	if questTracking := os.Getenv("TARKOV_ENABLE_QUEST_TRACKING"); questTracking != "" {
		config.EnableQuestTracking = questTracking == "true" || questTracking == "1"
	}

	if config.PartySettings.ClientID == "" {
		config.PartySettings.ClientID = uuid.New().String()
	}

	// RemoteID may be empty or a documented placeholder. The desktop app
	// auto-generates one on startup; failing here would discard every other
	// field from a loaded config.json (screenshot dirs, party identity,
	// tracker token) and the subsequent save would overwrite the file.

	if config.ScreenshotDir == "" {
		defaultDir, err := getDefaultScreenshotDir()
		if err != nil {
			return nil, fmt.Errorf("failed to find default screenshot directory: %w", err)
		}
		config.ScreenshotDir = defaultDir
	}

	return config, nil
}

func getDefaultScreenshotDir() (string, error) {
	if screenshotDir := os.Getenv("TARKOV_SCREENSHOT_DIR"); screenshotDir != "" {
		if _, err := os.Stat(screenshotDir); err == nil {
			return screenshotDir, nil
		}
		log.Printf("Environment variable TARKOV_SCREENSHOT_DIR path not found: %s", screenshotDir)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		paths := []string{
			filepath.Join(homeDir, "Documents", "Escape from Tarkov", "Screenshots"),
			filepath.Join(homeDir, "Pictures", "Escape from Tarkov"),
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		return paths[0], nil
	case "darwin", "linux":
		return filepath.Join(homeDir, "Documents", "Escape from Tarkov", "Screenshots"), nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// ShouldAutoStart reports whether integration should start on app launch.
// First-run and placeholder configs omit setupComplete, so a generated
// Remote ID must not start tracking before the wizard finishes.
func (c *Config) ShouldAutoStart() bool {
	if c == nil {
		return false
	}
	return c.AutoConnect && c.SetupComplete && c.RemoteID != "" && c.RemoteID != "your_remote_id_here"
}

func (c *Config) GetTarkovTrackerConfig() (baseURL, token string, enabled bool) {
	return c.TarkovTrackerURL, c.TarkovTrackerToken, c.EnableQuestTracking
}

func (c *Config) IsQuestTrackingConfigured() bool {
	return c.EnableQuestTracking && c.TarkovTrackerURL != "" && c.TarkovTrackerToken != ""
}
