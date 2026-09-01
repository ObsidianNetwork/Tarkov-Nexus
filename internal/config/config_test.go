package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir string, raw interface{}) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	var data []byte
	var err error
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	default:
		data, err = json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadConfigKeepsSettingsWhenRemoteIDMissing pins the upgrade path from
// the documented example config (`"remoteId": "your_remote_id_here"`) and from
// a saved file that never got a Remote ID. LoadConfig used to return an error
// *and* nil, so the app discarded screenshot dirs, party ClientID, and tracker
// token, then overwrote the file after auto-generating a Remote ID.
func TestLoadConfigKeepsSettingsWhenRemoteIDMissing(t *testing.T) {
	dir := t.TempDir()

	raw := map[string]interface{}{
		"remoteId":           "your_remote_id_here",
		"screenshotDir":      filepath.Join(dir, "screenshots"),
		"logsDir":            filepath.Join(dir, "logs"),
		"tarkovTrackerToken": "tracker-secret",
		"partySettings": map[string]interface{}{
			"clientId":    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			"displayName": "RaidBuddy",
			"enabled":     true,
			"serverUrl":   "wss://party.tarkov.nexus/ws",
		},
	}
	path := writeConfig(t, dir, raw)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config")
	}
	if cfg.ScreenshotDir != raw["screenshotDir"] {
		t.Errorf("ScreenshotDir = %q, want saved path", cfg.ScreenshotDir)
	}
	if cfg.LogsDir != raw["logsDir"] {
		t.Errorf("LogsDir = %q, want saved path", cfg.LogsDir)
	}
	if cfg.TarkovTrackerToken != "tracker-secret" {
		t.Errorf("TarkovTrackerToken = %q, want tracker-secret", cfg.TarkovTrackerToken)
	}
	if cfg.PartySettings.ClientID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("PartySettings.ClientID = %q, want persisted UUID", cfg.PartySettings.ClientID)
	}
	if cfg.PartySettings.DisplayName != "RaidBuddy" {
		t.Errorf("PartySettings.DisplayName = %q, want RaidBuddy", cfg.PartySettings.DisplayName)
	}
	if cfg.AutoConnect {
		t.Error("AutoConnect = true, want false when omitted so first-run does not auto-start")
	}
	if !cfg.DarkMode {
		t.Error("DarkMode = false, want true to match first-run defaults")
	}
	if cfg.ShouldAutoStart() {
		t.Error("placeholder config without setupComplete must not auto-start")
	}
}

func TestLoadConfigAllowsEmptyRemoteID(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `{"screenshotDir":"`+filepath.ToSlash(dir)+`"}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.RemoteID != "" {
		t.Errorf("RemoteID = %q, want empty so the app can auto-generate", cfg.RemoteID)
	}
}

func TestLoadConfigMissingFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded for a missing file; startup would keep AutoConnect=true instead of createDefaultConfig")
	}
	if cfg != nil {
		t.Fatal("LoadConfig should return nil config when the file is missing")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want os.ErrNotExist so startup can apply first-run defaults", err)
	}
}

func TestLoadConfigOmittedAutoConnectDoesNotDefaultTrue(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]interface{}{
		"remoteId":      "your_remote_id_here",
		"screenshotDir": dir,
	})

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.AutoConnect {
		t.Error("omitted autoConnect defaulted to true; placeholder configs would auto-start after Remote ID generation")
	}
	if !cfg.DarkMode {
		t.Error("omitted darkMode defaulted to false; want first-run DarkMode true")
	}
}

func TestLoadConfigPreservesExplicitAutoConnect(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]interface{}{
		"remoteId":      "abcd",
		"screenshotDir": dir,
		"autoConnect":   true,
		"setupComplete": true,
		"darkMode":      false,
	})

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.AutoConnect {
		t.Error("explicit autoConnect true was not preserved")
	}
	if !cfg.SetupComplete {
		t.Error("explicit setupComplete true was not preserved")
	}
	if cfg.DarkMode {
		t.Error("explicit darkMode false was not preserved")
	}
	if !cfg.ShouldAutoStart() {
		t.Error("configured app with autoConnect and setupComplete should auto-start")
	}
}

func TestDefaultConfigMatchesFirstRun(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AutoConnect {
		t.Error("DefaultConfig AutoConnect = true, want false")
	}
	if !cfg.DarkMode {
		t.Error("DefaultConfig DarkMode = false, want true")
	}
	if cfg.ShouldAutoStart() {
		t.Error("DefaultConfig must not auto-start")
	}
}

func TestShouldAutoStart(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{name: "nil", cfg: nil, want: false},
		{name: "first-run defaults", cfg: DefaultConfig(), want: false},
		{
			name: "generated id before setup",
			cfg:  &Config{AutoConnect: true, RemoteID: "Ab12", SetupComplete: false},
			want: false,
		},
		{
			name: "placeholder remote id",
			cfg:  &Config{AutoConnect: true, RemoteID: "your_remote_id_here", SetupComplete: true},
			want: false,
		},
		{
			name: "empty remote id",
			cfg:  &Config{AutoConnect: true, RemoteID: "", SetupComplete: true},
			want: false,
		},
		{
			name: "setup complete but autoConnect off",
			cfg:  &Config{AutoConnect: false, RemoteID: "Ab12", SetupComplete: true},
			want: false,
		},
		{
			name: "ready",
			cfg:  &Config{AutoConnect: true, RemoteID: "Ab12", SetupComplete: true},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ShouldAutoStart(); got != tt.want {
				t.Errorf("ShouldAutoStart() = %v, want %v", got, tt.want)
			}
		})
	}
}
