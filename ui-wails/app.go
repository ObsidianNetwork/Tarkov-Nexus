package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/config"
	"tarkov-screenshot-analyzer/internal/game"
	"tarkov-screenshot-analyzer/internal/integration"
	"tarkov-screenshot-analyzer/internal/overlay"
	"tarkov-screenshot-analyzer/internal/party"
	"tarkov-screenshot-analyzer/internal/position"
	"tarkov-screenshot-analyzer/internal/tracker"
	"tarkov-screenshot-analyzer/internal/updater"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds the application state
type App struct {
	ctx              context.Context
	integration      *integration.Integration
	config           *config.Config
	logger           *Logger
	updater          *updater.Updater
	updateChecker    *updater.BackgroundChecker
	partyServer      *party.Server
	overlayServer    *overlay.Server
	isHostingParty   bool
	currentPartyCode string
	partyMembers     []map[string]interface{} // Track party members when joined as client
	stateMutex       sync.RWMutex
	isRunning        bool
	dataDir          string // AppData directory for config/logs
}

// Logger maintains an in-memory log buffer
type Logger struct {
	entries    []LogEntry
	mutex      sync.RWMutex
	maxEntries int
}

// LogEntry represents a single log message
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// NewLogger creates a new in-memory logger
func NewLogger(maxEntries int) *Logger {
	return &Logger{
		entries:    make([]LogEntry, 0),
		maxEntries: maxEntries,
	}
}

// Add adds a log entry
func (l *Logger) Add(level, message string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Message:   message,
	}

	l.entries = append(l.entries, entry)

	// Keep only the last N entries
	if len(l.entries) > l.maxEntries {
		l.entries = l.entries[len(l.entries)-l.maxEntries:]
	}
}

// GetAll returns all log entries
func (l *Logger) GetAll() []LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// Return a copy
	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

// Clear removes all log entries
func (l *Logger) Clear() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.entries = make([]LogEntry, 0)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.Add("INFO", msg)
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.Add("DEBUG", msg)
}

// Warning logs a warning message
func (l *Logger) Warning(msg string) {
	l.Add("WARNING", msg)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.Add("ERROR", msg)
}

// NewApp creates a new App application struct.
// dataDir is the directory for config.json and logs (typically %AppData%/TarkovNexus).
func NewApp(dataDir string) *App {
	return &App{
		logger:  NewLogger(500), // Keep last 500 log entries
		dataDir: dataDir,
	}
}

// configFilePath returns the full path to config.json inside the data directory.
func (a *App) configFilePath() string {
	if a.dataDir == "" {
		return "config.json"
	}
	return filepath.Join(a.dataDir, "config.json")
}

// generateRemoteID creates a random 4-character alphanumeric ID,
// matching the format tarkov.dev uses (makeID(4)).
func generateRemoteID() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 4)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

// shortID safely truncates an ID for logging.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logInfo("Application started")
	if a.dataDir != "" {
		a.logInfo(fmt.Sprintf("Data directory: %s", a.dataDir))
	}

	// Load configuration
	cfg, err := config.LoadConfig(a.configFilePath())
	if err != nil {
		a.logError(fmt.Sprintf("Failed to load configuration: %v", err))
		// Create default config
		cfg = a.createDefaultConfig()
	} else {
		a.logInfo("Configuration loaded successfully")
	}
	a.config = cfg

	// Auto-generate a Remote ID on first boot so the map works immediately
	if a.config.RemoteID == "" || a.config.RemoteID == "your_remote_id_here" {
		a.config.RemoteID = generateRemoteID()
		a.logInfo(fmt.Sprintf("Generated Remote ID: %s", a.config.RemoteID))
		if err := a.SaveConfig(a.config); err != nil {
			a.logError(fmt.Sprintf("Failed to save generated Remote ID: %v", err))
		}
	}

	// Initialize updater
	a.initializeUpdater()

	// Start overlay server
	a.overlayServer = overlay.NewServer(44444, a.logger)
	if err := a.overlayServer.Start(); err != nil {
		a.logError(fmt.Sprintf("Failed to start overlay server: %v", err))
	}
	// Tell the map viewer which Remote ID is "self" so it can highlight the player's own marker
	if a.config.RemoteID != "" && a.config.RemoteID != "your_remote_id_here" {
		a.overlayServer.SetSelfRemoteID(a.config.RemoteID)
	}

	// When the map window reports a session ID, update the config automatically
	a.overlayServer.OnRemoteIDCaptured(func(remoteID string) {
		a.stateMutex.Lock()
		old := a.config.RemoteID
		a.config.RemoteID = remoteID
		a.stateMutex.Unlock()

		if old != remoteID {
			a.logInfo(fmt.Sprintf("Remote ID updated from map: %s", remoteID))
			a.overlayServer.SetSelfRemoteID(remoteID)
			// Persist to disk
			if err := a.SaveConfig(a.config); err != nil {
				a.logError(fmt.Sprintf("Failed to save captured Remote ID: %v", err))
			}
			a.emitEvent("config:remoteIdUpdated", remoteID)
		}
	})

	// Auto-start only after the setup wizard has finished. LoadConfig no
	// longer fails on a missing Remote ID, so a generated ID plus omitted
	// autoConnect must not start integration during first-run setup.
	if a.config.ShouldAutoStart() {
		a.logInfo("Auto-connect enabled, starting integration...")
		if err := a.StartIntegration(); err != nil {
			a.logError(fmt.Sprintf("Failed to auto-start integration: %v", err))
		}
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.logInfo("Application shutting down")
	if a.isRunning {
		a.StopIntegration()
	}
	if a.updateChecker != nil {
		a.updateChecker.Stop()
	}
	if a.partyServer != nil {
		a.partyServer.Stop()
	}
	if a.overlayServer != nil {
		a.overlayServer.Stop()
	}
}

// domReady is called after the front-end dom has been loaded
func (a *App) domReady(ctx context.Context) {
	// Optional: Perform any post-load initialization
}

// beforeClose is called before the application terminates
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// Return false to allow close, true to prevent
	return false
}

// createDefaultConfig creates a default configuration
func (a *App) createDefaultConfig() *config.Config {
	return config.DefaultConfig()
}

// ===== CONFIGURATION METHODS =====

// GetConfig returns the current configuration
func (a *App) GetConfig() *config.Config {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()
	return a.config
}

// GetDataDir returns the application data directory path (for display in Settings/About).
func (a *App) GetDataDir() string {
	if a.dataDir == "" {
		return "." // CWD fallback
	}
	return a.dataDir
}

// SaveConfig saves the configuration to disk
func (a *App) SaveConfig(cfg *config.Config) error {
	a.stateMutex.Lock()
	a.config = cfg
	a.stateMutex.Unlock()

	// Save configuration to config.json in the data directory
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		a.logError(fmt.Sprintf("Failed to marshal configuration: %v", err))
		return err
	}

	cfgPath := a.configFilePath()
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		a.logError(fmt.Sprintf("Failed to write config file: %v", err))
		return err
	}

	a.logInfo("Configuration saved successfully")

	// Keep map viewer aware of the self Remote ID in case it changed
	if a.overlayServer != nil && cfg.RemoteID != "" && cfg.RemoteID != "your_remote_id_here" {
		a.overlayServer.SetSelfRemoteID(cfg.RemoteID)
	}
	return nil
}

// GetDefaultScreenshotDir attempts to find the default screenshot directory
func (a *App) GetDefaultScreenshotDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
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
	}

	return filepath.Join(homeDir, "Documents", "Escape from Tarkov", "Screenshots"), nil
}

// GetDefaultLogsDir attempts to find the default logs directory
func (a *App) GetDefaultLogsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		// Try to find the game installation directory using multiple methods

		// Method 1: Parse BSG Launcher logs for gamesRootDir setting
		if logsDir := a.findLogsFromLauncherSettings(homeDir); logsDir != "" {
			return logsDir, nil
		}

		// Method 2: Search common installation paths
		commonPaths := []string{
			// Common custom installation roots
			"C:\\Tarkov\\Logs",
			"D:\\Tarkov\\Logs",
			"E:\\Tarkov\\Logs",
			"C:\\Games\\Tarkov\\Logs",
			"D:\\Games\\Tarkov\\Logs",
			"C:\\Games\\EscapeFromTarkov\\Logs",
			"D:\\Games\\EscapeFromTarkov\\Logs",
			// BSG default installation paths
			"C:\\Battlestate Games\\EFT\\Logs",
			"D:\\Battlestate Games\\EFT\\Logs",
			"C:\\Battlestate Games\\EFT (live)\\Logs",
			"D:\\Battlestate Games\\EFT (live)\\Logs",
			// Program Files paths (less common)
			"C:\\Program Files\\Escape From Tarkov\\Logs",
			"C:\\Program Files (x86)\\Escape From Tarkov\\Logs",
		}

		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		// Method 3: Search drive roots for Tarkov installations
		if logsDir := a.searchDrivesForTarkovLogs(); logsDir != "" {
			return logsDir, nil
		}

		// Fallback: Return empty string to indicate auto-detect failed
		// User will need to manually browse
		return "", nil
	}

	return filepath.Join(homeDir, ".config", "EscapeFromTarkov", "Logs"), nil
}

// findLogsFromLauncherSettings parses BSG Launcher logs to find gamesRootDir
func (a *App) findLogsFromLauncherSettings(homeDir string) string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(homeDir, "AppData", "Local")
	}

	launcherLogsDir := filepath.Join(localAppData, "Battlestate Games", "BsgLauncher", "Logs")

	// Find the most recent launcher log file
	entries, err := os.ReadDir(launcherLogsDir)
	if err != nil {
		return ""
	}

	var newestLog string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "BSG_Launcher_") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestLog = filepath.Join(launcherLogsDir, entry.Name())
		}
	}

	if newestLog == "" {
		return ""
	}

	// Read and parse the log file for gamesRootDir
	content, err := os.ReadFile(newestLog)
	if err != nil {
		return ""
	}

	// Look for gamesRootDir in the settings JSON
	// Pattern: "gamesRootDir":"C:\\Battlestate Games"
	contentStr := string(content)
	gamesRootDirPrefix := `"gamesRootDir":"`
	idx := strings.Index(contentStr, gamesRootDirPrefix)
	if idx == -1 {
		return ""
	}

	// Extract the path
	start := idx + len(gamesRootDirPrefix)
	end := strings.Index(contentStr[start:], `"`)
	if end == -1 {
		return ""
	}

	gamesRootDir := contentStr[start : start+end]
	// Unescape the path (JSON escapes backslashes)
	gamesRootDir = strings.ReplaceAll(gamesRootDir, `\\`, `\`)

	// Check for EFT installation in the games root
	possiblePaths := []string{
		filepath.Join(gamesRootDir, "EFT", "Logs"),
		filepath.Join(gamesRootDir, "EFT (live)", "Logs"),
		filepath.Join(gamesRootDir, "Escape From Tarkov", "Logs"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// searchDrivesForTarkovLogs searches common drive letters for Tarkov installations
func (a *App) searchDrivesForTarkovLogs() string {
	drives := []string{"C:", "D:", "E:", "F:", "G:"}

	for _, drive := range drives {
		// Check if drive exists
		if _, err := os.Stat(drive + "\\"); err != nil {
			continue
		}

		// Search for common Tarkov folder names at drive root
		tarkovNames := []string{"Tarkov", "EFT", "EscapeFromTarkov", "Escape From Tarkov"}
		for _, name := range tarkovNames {
			logsPath := filepath.Join(drive+"\\", name, "Logs")
			if _, err := os.Stat(logsPath); err == nil {
				// Verify it's actually Tarkov logs by checking for log files
				entries, err := os.ReadDir(logsPath)
				if err == nil && len(entries) > 0 {
					return logsPath
				}
			}
		}

		// Also check Battlestate Games folder on each drive
		bsgPath := filepath.Join(drive+"\\", "Battlestate Games")
		if _, err := os.Stat(bsgPath); err == nil {
			eftPaths := []string{
				filepath.Join(bsgPath, "EFT", "Logs"),
				filepath.Join(bsgPath, "EFT (live)", "Logs"),
			}
			for _, path := range eftPaths {
				if _, err := os.Stat(path); err == nil {
					return path
				}
			}
		}
	}

	return ""
}

// ===== INTEGRATION CONTROL METHODS =====

// StartIntegration initializes and starts the integration
func (a *App) StartIntegration() error {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()

	if a.isRunning {
		return fmt.Errorf("integration is already running")
	}

	// Validate configuration
	if a.config.RemoteID == "" || a.config.RemoteID == "your_remote_id_here" {
		return fmt.Errorf("remote ID not configured")
	}

	if a.config.ScreenshotDir == "" {
		return fmt.Errorf("screenshot directory not configured")
	}

	a.logInfo("Starting integration...")

	// Create integration instance (pass App as logger since it implements logger.Logger interface)
	a.integration = integration.New(a.config, a)

	// Pass overlay server to integration if it exists
	if a.overlayServer != nil {
		a.integration.SetOverlayServer(a.overlayServer)
	}

	// Setup event handlers to emit events to frontend
	a.setupIntegrationEventHandlers()

	// Initialize integration
	if err := a.integration.Initialize(); err != nil {
		a.logError(fmt.Sprintf("Failed to initialize integration: %v", err))
		return err
	}

	a.isRunning = true
	a.logInfo("Integration started successfully")
	a.emitEvent("integration:started", nil)

	return nil
}

// StopIntegration stops the integration
func (a *App) StopIntegration() error {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()

	if !a.isRunning {
		return fmt.Errorf("integration is not running")
	}

	a.logInfo("Stopping integration...")

	if a.integration != nil {
		if err := a.integration.Shutdown(); err != nil {
			a.logError(fmt.Sprintf("Error during shutdown: %v", err))
		}
		a.integration = nil
	}

	a.isRunning = false
	a.logInfo("Integration stopped")
	a.emitEvent("integration:stopped", nil)

	return nil
}

// GetStatus returns the current integration status
func (a *App) GetStatus() map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	status := map[string]interface{}{
		"isRunning":        a.isRunning,
		"isConfigured":     a.config.RemoteID != "" && a.config.RemoteID != "your_remote_id_here",
		"hasScreenshotDir": a.config.ScreenshotDir != "",
		"hasLogsDir":       a.config.LogsDir != "",
	}

	// If integration is running, get detailed status
	if a.isRunning && a.integration != nil {
		integrationStatus := a.integration.GetStatus()
		for key, value := range integrationStatus {
			status[key] = value
		}
	} else {
		// Provide default values when not running
		status["initialized"] = false
		status["connected"] = false
		status["monitoring"] = false
		status["currentMap"] = ""
		status["currentNormalizedMap"] = ""
		status["lastPosition"] = nil
		status["lastUpdate"] = ""
		status["screenshot"] = map[string]interface{}{
			"isMonitoring":   false,
			"processedCount": 0,
			"pendingCount":   0,
		}
		status["websocket"] = map[string]interface{}{
			"totalReconnects":  0,
			"lastPingTime":     "",
			"connectionUptime": "",
		}
		status["questTracking"] = map[string]interface{}{
			"enabled":    a.config.EnableQuestTracking,
			"configured": a.config.TarkovTrackerToken != "",
		}
	}

	return status
}

// IsRunning returns whether the integration is currently running
func (a *App) IsRunning() bool {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()
	return a.isRunning
}

// ===== FILESYSTEM PICKER METHODS =====

// SelectScreenshotDirectory opens a directory picker for screenshot directory
func (a *App) SelectScreenshotDirectory() (string, error) {
	options := wailsRuntime.OpenDialogOptions{
		Title: "Select Screenshot Directory",
	}

	// Set default directory if configured
	if a.config.ScreenshotDir != "" {
		options.DefaultDirectory = a.config.ScreenshotDir
	}

	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, options)
	if err != nil {
		return "", err
	}

	return dir, nil
}

// SelectLogsDirectory opens a directory picker for logs directory
func (a *App) SelectLogsDirectory() (string, error) {
	options := wailsRuntime.OpenDialogOptions{
		Title: "Select Logs Directory",
	}

	// Set default directory if configured
	if a.config.LogsDir != "" {
		options.DefaultDirectory = a.config.LogsDir
	}

	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, options)
	if err != nil {
		return "", err
	}

	return dir, nil
}

// ===== MAP METHODS =====

// GetAvailableMaps returns the list of maps offered by the manual selector.
//
// The list itself lives in internal/game, derived from the map resolver, so the
// selector cannot offer something the resolver and the tarkov.dev allowlist
// would reject. This binding is only the adapter to the frontend's shape.
func (a *App) GetAvailableMaps() []map[string]string {
	selectable := game.NewMapResolver().GetSelectableMaps()

	maps := make([]map[string]string, 0, len(selectable))
	for _, m := range selectable {
		maps = append(maps, map[string]string{
			"id":          m.NormalizedName,
			"displayName": m.DisplayName,
		})
	}

	return maps
}

// SetManualMapOverride sets a manual map override
func (a *App) SetManualMapOverride(mapName string) error {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	if !a.isRunning || a.integration == nil {
		return fmt.Errorf("integration is not running")
	}

	a.integration.SetManualMapOverride(mapName)
	a.logInfo(fmt.Sprintf("Manual map override set to: %s", mapName))
	a.emitEvent("map:manualOverride", map[string]string{"map": mapName})

	return nil
}

// ClearManualMapOverride clears the manual map override
func (a *App) ClearManualMapOverride() error {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	if !a.isRunning || a.integration == nil {
		return fmt.Errorf("integration is not running")
	}

	a.integration.SetManualMapOverride("")
	a.logDebug("Manual map override cleared")
	a.emitEvent("map:manualOverride", map[string]string{"map": ""})

	return nil
}

// InjectPosition allows manual injection of position updates for testing
func (a *App) InjectPosition(x, y, z, rotation float64, mapName string) error {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	if !a.isRunning || a.integration == nil {
		return fmt.Errorf("integration is not running")
	}

	a.logInfo(fmt.Sprintf("💉 Injecting position: %.2f, %.2f, %.2f on %s", x, y, z, mapName))

	// Create position data
	pos := &position.PositionData{
		X:        x,
		Y:        y,
		Z:        z,
		Rotation: rotation,
		Map:      mapName,
	}

	// Directly call update position on integration
	// Note: We need to expose a method on Integration to handle this or make UpdatePosition public
	// For now, let's assume we can add a method to Integration or use an existing one.
	// Since UpdatePosition is private in Integration, we should add a public method there too.
	// But wait, we can't modify Integration easily without checking it first.
	// Let's check Integration.go again.

	// Actually, looking at integration.go earlier, updatePosition is private.
	// We should add a public method to Integration to handle manual updates.
	// For now, I will add the method call and then update Integration.go.

	// Forward to centralized party if connected
	a.SendPositionToParty(mapName, x, y, z, rotation)

	return a.integration.InjectPosition(pos)
}

// ===== PARTY METHODS =====

// StartPartyServer starts the party server and returns a party code
func (a *App) StartPartyServer() (string, error) {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()

	// Log immediately at function entry
	a.logInfo("🎮 StartPartyServer() called")

	// Check if party features are enabled
	if !a.config.PartySettings.Enabled {
		a.logError("❌ Cannot start party: Party features are disabled in settings")
		a.logInfo("💡 Enable party features in Settings → Party Settings first")
		return "", fmt.Errorf("party features are disabled - enable in Settings first")
	}
	a.logInfo("✓ Party features enabled check passed")

	// Check if already hosting
	if a.isHostingParty {
		a.logWarning("⚠️ Already hosting a party with code: " + a.currentPartyCode)
		return "", fmt.Errorf("already hosting a party")
	}
	a.logInfo("✓ Not currently hosting a party")

	// Create and start party server
	a.partyServer = party.NewServer(a.config.PartySettings.ServerPort, a)
	if err := a.partyServer.Start(); err != nil {
		a.logError(fmt.Sprintf("Failed to start party server: %v", err))
		return "", err
	}

	// Get display name (with fallback)
	displayName := a.config.PartySettings.DisplayName
	if displayName == "" {
		displayName = "Host"
	}

	// Create a party and get the code (host is auto-added as first member)
	partyCode, err := a.partyServer.CreateParty(a.config.RemoteID, displayName)
	if err != nil {
		a.partyServer.Stop()
		a.partyServer = nil
		a.logError(fmt.Sprintf("Failed to create party: %v", err))
		return "", err
	}

	// Update hosting state
	a.isHostingParty = true
	a.currentPartyCode = partyCode

	a.logInfo(fmt.Sprintf("🎮 Party server started with code: %s", partyCode))

	// Host needs to connect to their own party server as a client
	// This allows the host to send/receive position updates
	a.logInfo("Connecting host to their own party server...")
	serverURL := fmt.Sprintf("ws://localhost:%d/party", a.config.PartySettings.ServerPort)

	// Connect to own server (integration handles this)
	if a.isRunning && a.integration != nil {
		if err := a.integration.ConnectToParty(serverURL, partyCode, displayName); err != nil {
			a.logError(fmt.Sprintf("Failed to connect host to own party: %v", err))
			// Don't fail - server is still running
		} else {
			a.logInfo("✅ Host connected to own party server")
		}
	}

	a.emitEvent("party:serverStarted", map[string]string{"partyCode": partyCode})

	return partyCode, nil
}

// StopPartyServer stops the party server
func (a *App) StopPartyServer() error {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()

	if !a.isHostingParty || a.partyServer == nil {
		return fmt.Errorf("not hosting a party")
	}

	a.logInfo("Stopping party server...")

	// Disconnect from own party if hosting
	if a.isRunning && a.integration != nil {
		if err := a.integration.DisconnectFromParty(); err != nil {
			a.logWarning(fmt.Sprintf("Failed to disconnect from party: %v", err))
		}
	}

	if err := a.partyServer.Stop(); err != nil {
		a.logError(fmt.Sprintf("Failed to stop party server: %v", err))
		return err
	}

	a.partyServer = nil
	a.isHostingParty = false
	a.currentPartyCode = ""

	a.emitEvent("party:serverStopped", nil)
	a.logInfo("✅ Party server stopped")

	return nil
}

// JoinParty connects to a party server and joins a party
func (a *App) JoinParty(serverURL, partyCode string) error {
	// Log immediately at function entry
	a.logInfo(fmt.Sprintf("🎮 JoinParty() called - Server: %s, Code: %s", serverURL, partyCode))

	// Acquire read lock for validation checks only
	a.stateMutex.RLock()

	// Check if party features are enabled
	if !a.config.PartySettings.Enabled {
		a.stateMutex.RUnlock()
		a.logError("❌ Cannot join party: Party features are disabled in settings")
		a.logInfo("💡 Enable party features in Settings → Party Settings first")
		return fmt.Errorf("party features are disabled - enable in Settings first")
	}
	a.logInfo("✓ Party features enabled check passed")

	// Check if integration is running
	if !a.isRunning || a.integration == nil {
		a.stateMutex.RUnlock()
		a.logError("❌ Cannot join party: Integration service is not running")
		a.logInfo("💡 Start the Integration service on the Dashboard before joining a party")
		return fmt.Errorf("integration service must be running - start it on Dashboard first")
	}
	a.logInfo("✓ Integration service running check passed")

	// Get display name while holding lock
	displayName := a.config.PartySettings.DisplayName
	if displayName == "" {
		displayName = "Player"
	}
	a.logInfo(fmt.Sprintf("Using display name: %s", displayName))

	// IMPORTANT: Release read lock before calling ConnectToParty
	// This prevents deadlock when SaveConfig needs write lock later
	a.stateMutex.RUnlock()

	// Connect to party (no lock held - allows other operations to proceed)
	if err := a.integration.ConnectToParty(serverURL, partyCode, displayName); err != nil {
		a.logError(fmt.Sprintf("Failed to join party: %v", err))
		return err
	}

	// Acquire write lock to update config
	a.stateMutex.Lock()
	a.config.PartySettings.PartyCode = partyCode
	a.stateMutex.Unlock()

	// Save config (handles its own locking)
	if err := a.SaveConfig(a.config); err != nil {
		a.logWarning(fmt.Sprintf("Failed to save party code to config: %v", err))
	}

	a.logInfo(fmt.Sprintf("🎮 Joined party: %s", partyCode))
	a.emitEvent("party:joined", map[string]string{"partyCode": partyCode})

	return nil
}

// LeaveParty disconnects from the current party
func (a *App) LeaveParty() error {
	// Acquire read lock for validation check only
	a.stateMutex.RLock()

	if !a.isRunning || a.integration == nil {
		a.stateMutex.RUnlock()
		return fmt.Errorf("integration is not running")
	}

	// Release read lock before calling DisconnectFromParty
	a.stateMutex.RUnlock()

	// Disconnect from party (no lock held)
	if err := a.integration.DisconnectFromParty(); err != nil {
		a.logError(fmt.Sprintf("Failed to leave party: %v", err))
		return err
	}

	// Acquire write lock to update config
	a.stateMutex.Lock()
	a.config.PartySettings.PartyCode = ""
	a.stateMutex.Unlock()

	// Save config (handles its own locking)
	if err := a.SaveConfig(a.config); err != nil {
		a.logWarning(fmt.Sprintf("Failed to clear party code from config: %v", err))
	}

	a.logInfo("👋 Left party")
	a.emitEvent("party:left", nil)

	return nil
}

// GetPartyStatus returns the current party status
func (a *App) GetPartyStatus() map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	status := map[string]interface{}{
		"active":      false,
		"isHost":      false,
		"partyCode":   "",
		"memberCount": 0,
		"enabled":     a.config.PartySettings.Enabled,
	}

	// Check if hosting
	if a.isHostingParty && a.partyServer != nil {
		status["active"] = true
		status["isHost"] = true
		status["partyCode"] = a.currentPartyCode

		// Get party from server to count members
		if party, exists := a.partyServer.GetPartyStatus(a.currentPartyCode); exists {
			status["memberCount"] = party.GetMemberCount()
		}
	}

	// Check if joined as member
	if a.isRunning && a.integration != nil && a.integration.IsInParty() {
		status["active"] = true
		status["isHost"] = false
		status["partyCode"] = a.integration.GetPartyCode()

		// Use tracked party members for count
		// Add 1 to include self if not in the list, or just use list length if it includes self
		// Based on event handlers, partyMembers includes everyone including self if received from welcome/updates
		status["memberCount"] = len(a.partyMembers)
	}

	return status
}

// GetPublicIP returns the public IP address
func (a *App) GetPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org?format=text")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(ip), nil
}

// GetPartyMembers returns the list of party members
func (a *App) GetPartyMembers() []map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	members := []map[string]interface{}{}

	// If hosting, get members from party server
	if a.isHostingParty && a.partyServer != nil {
		if party, exists := a.partyServer.GetPartyStatus(a.currentPartyCode); exists {
			partyMembers := party.GetMembers()
			for _, m := range partyMembers {
				member := map[string]interface{}{
					"remoteId":    m.RemoteID,
					"displayName": m.DisplayName,
					"connected":   m.Connected,
					"currentMap":  m.CurrentMap,
					"lastUpdate":  m.LastUpdate.Format(time.RFC3339),
				}
				if m.LastPosition != nil {
					member["lastPosition"] = map[string]interface{}{
						"x":        m.LastPosition.X,
						"y":        m.LastPosition.Y,
						"z":        m.LastPosition.Z,
						"rotation": m.LastPosition.Rotation,
					}
				}
				members = append(members, member)
			}
		}
	}

	// If joined as member, return tracked members
	if !a.isHostingParty && len(a.partyMembers) > 0 {
		return a.partyMembers
	}

	return members
}

// GetPartyServerInfo returns detailed server information for hosting
func (a *App) GetPartyServerInfo() map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	info := map[string]interface{}{
		"isHosting":       a.isHostingParty,
		"port":            a.config.PartySettings.ServerPort,
		"ips":             []string{},
		"urls":            []string{},
		"preferredIP":     "",
		"preferredURL":    "",
		"portAvailable":   true,
		"firewallWarning": false,
	}

	// Get local IP addresses
	ips, err := party.GetLocalIPAddresses()
	if err != nil {
		a.logError(fmt.Sprintf("Failed to get local IP addresses: %v", err))
		return info
	}
	info["ips"] = ips

	// Get WebSocket URLs
	urls, err := party.GetWebSocketURLs(a.config.PartySettings.ServerPort)
	if err != nil {
		a.logError(fmt.Sprintf("Failed to generate WebSocket URLs: %v", err))
		return info
	}
	info["urls"] = urls

	// Get preferred IP and URL
	preferredIP, err := party.GetPreferredIP()
	if err == nil {
		info["preferredIP"] = preferredIP
		info["preferredURL"] = fmt.Sprintf("ws://%s:%d/party", preferredIP, a.config.PartySettings.ServerPort)
	}

	// Check port availability (only if not hosting)
	if !a.isHostingParty {
		portAvailable := party.IsPortAvailable(a.config.PartySettings.ServerPort)
		info["portAvailable"] = portAvailable

		// Show firewall warning if port appears to be blocked
		if !portAvailable {
			info["firewallWarning"] = true
		}
	}

	return info
}

// ===== QUEST TRACKING METHODS =====

// ValidateQuestToken validates the TarkovTracker token
func (a *App) ValidateQuestToken() error {
	if a.config.TarkovTrackerToken == "" {
		return fmt.Errorf("no token configured")
	}

	client := tracker.NewClient(tracker.Config{
		BaseURL: a.config.TarkovTrackerURL,
		Token:   a.config.TarkovTrackerToken,
	})
	defer client.Close()

	isValid, err := client.IsValidToken()
	if err != nil {
		a.logError(fmt.Sprintf("Token validation failed: %v", err))
		return err
	}

	if !isValid {
		a.logError("Token is invalid")
		return fmt.Errorf("token is invalid")
	}

	a.logInfo("TarkovTracker token validated successfully")
	return nil
}

// GetQuestProgress retrieves user quest progress from TarkovTracker
func (a *App) GetQuestProgress() (interface{}, error) {
	if a.config.TarkovTrackerToken == "" {
		return nil, fmt.Errorf("no token configured")
	}

	client := tracker.NewClient(tracker.Config{
		BaseURL: a.config.TarkovTrackerURL,
		Token:   a.config.TarkovTrackerToken,
	})
	defer client.Close()

	// Get gameMode from config, default to "pvp" if not set
	gameMode := a.config.TarkovTrackerGameMode
	if gameMode == "" {
		gameMode = "pvp"
	}

	a.logDebug(fmt.Sprintf("Fetching quest progress for game mode: %s", gameMode))
	progress, err := client.GetProgress(gameMode)
	if err != nil {
		a.logError(fmt.Sprintf("Failed to fetch quest progress: %v", err))
		return nil, err
	}

	// Log quest stats for debugging
	totalTasks := len(progress.Data.TasksProgress)
	validTasks := 0
	completedTasks := 0
	inProgressTasks := 0
	failedTasks := 0

	for _, task := range progress.Data.TasksProgress {
		if !task.Invalid {
			validTasks++
			if task.Complete && !task.Failed {
				completedTasks++
			} else if !task.Complete && !task.Failed {
				inProgressTasks++
			}
			if task.Failed {
				failedTasks++
			}
		}
	}

	a.logDebug(fmt.Sprintf("Quest stats for %s mode: Total=%d, Valid=%d, Completed=%d, InProgress=%d, Failed=%d",
		gameMode, totalTasks, validTasks, completedTasks, inProgressTasks, failedTasks))

	return progress, nil
}

// ===== LOGGING METHODS =====

// GetLogs returns all log entries
func (a *App) GetLogs() []LogEntry {
	return a.logger.GetAll()
}

// GetLogStats returns log statistics
func (a *App) GetLogStats() map[string]int {
	entries := a.logger.GetAll()
	stats := map[string]int{
		"total":   len(entries),
		"info":    0,
		"warning": 0,
		"error":   0,
		"debug":   0,
	}

	for _, entry := range entries {
		switch strings.ToLower(entry.Level) {
		case "info":
			stats["info"]++
		case "warning":
			stats["warning"]++
		case "error":
			stats["error"]++
		case "debug":
			stats["debug"]++
		}
	}

	return stats
}

// ClearLogs clears all log entries
func (a *App) ClearLogs() {
	a.logger.Clear()
	a.logInfo("Logs cleared")
}

// ExportLogs exports logs to a file
func (a *App) ExportLogs() (string, error) {
	entries := a.logger.GetAll()

	// Create logs directory inside the data directory
	logsDir := "logs"
	if a.dataDir != "" {
		logsDir = filepath.Join(a.dataDir, "logs")
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", err
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("tarkov-map-sync-%s.log", time.Now().Format("2006-01-02-150405"))
	filepath := filepath.Join(logsDir, filename)

	// Write logs to file
	var sb strings.Builder
	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("[%s] [%s] %s\n", entry.Timestamp, entry.Level, entry.Message))
	}

	if err := os.WriteFile(filepath, []byte(sb.String()), 0644); err != nil {
		return "", err
	}

	a.logInfo(fmt.Sprintf("Logs exported to: %s", filepath))
	return filepath, nil
}

// ===== UTILITY METHODS =====

// openTarkovDevMap opens tarkov.dev in the system browser on the current map with the Remote ID.
// Used by both the dev-mode OpenMapWindow and as a fallback in production.
func (a *App) openTarkovDevMap() error {
	url := "https://tarkov.dev"
	if a.config.RemoteID != "" && a.config.RemoteID != "your_remote_id_here" {
		status := a.GetStatus()
		mapPath := ""
		if m, ok := status["currentNormalizedMap"].(string); ok && m != "" {
			mapPath = m
		} else if m, ok := status["currentMap"].(string); ok && m != "" {
			mapPath = m
		}
		if mapPath != "" {
			url = fmt.Sprintf("https://tarkov.dev/map/%s?connection=%s", mapPath, a.config.RemoteID)
		} else {
			url = fmt.Sprintf("https://tarkov.dev/?connection=%s", a.config.RemoteID)
		}
	}
	wailsRuntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// OpenTarkovDevBrowser opens tarkov.dev in the default browser
func (a *App) OpenTarkovDevBrowser() error {
	wailsRuntime.BrowserOpenURL(a.ctx, "https://tarkov.dev")
	a.logInfo("Opened tarkov.dev in browser")
	return nil
}

// TestConnection tests the WebSocket connection
func (a *App) TestConnection() error {
	if a.config.RemoteID == "" || a.config.RemoteID == "your_remote_id_here" {
		return fmt.Errorf("remote ID not configured")
	}

	// This is a simple test - in production you might want more sophisticated testing
	a.logInfo("Testing connection...")

	if !a.isRunning {
		return fmt.Errorf("integration is not running - start the integration to test connection")
	}

	status := a.GetStatus()
	if connected, ok := status["connected"].(bool); ok && connected {
		a.logInfo("Connection test successful - WebSocket is connected")
		return nil
	}

	return fmt.Errorf("WebSocket is not connected")
}

// GetPlatform returns the current platform
func (a *App) GetPlatform() string {
	return runtime.GOOS
}

// GetVersion returns the current application version
func (a *App) GetVersion() string {
	return updater.Version
}

// ===== AUTO-UPDATE METHODS =====

// initializeUpdater initializes the updater and background checker
func (a *App) initializeUpdater() {
	// Determine update channel
	channel := updater.ChannelStable
	if a.config.UpdateSettings.UpdateChannel == "beta" {
		channel = updater.ChannelBeta
	}

	// Create updater
	a.updater = updater.NewUpdater(a, channel)

	// Setup update event handlers
	a.setupUpdateEventHandlers()

	// Always create the background checker so SetAutoUpdateCheck works at runtime.
	// If auto-check is disabled in config, the checker is created but not started.
	checkInterval := time.Duration(a.config.UpdateSettings.CheckInterval) * time.Hour
	a.updateChecker = updater.NewBackgroundChecker(a.updater, a, checkInterval)

	a.updateChecker.OnUpdateFound(func(info *updater.UpdateInfo) {
		a.logInfo(fmt.Sprintf("Update available: %s", info.Version))
		a.emitEvent("update:available", info)
	})

	if a.config.UpdateSettings.EnableAutoCheck {
		a.updateChecker.Start()
		a.logDebug("Background update checker started")
	} else {
		a.updateChecker.SetEnabled(false)
		a.logDebug("Background update checker created (disabled)")
	}
}

// setupUpdateEventHandlers sets up event handlers from updater to forward to frontend
func (a *App) setupUpdateEventHandlers() {
	if a.updater == nil {
		return
	}

	// Forward updater events to Wails events
	a.updater.OnEvent("update:available", func(data interface{}) {
		a.emitEvent("update:available", data)
	})

	a.updater.OnEvent("update:downloading", func(data interface{}) {
		a.emitEvent("update:downloading", data)
	})

	a.updater.OnEvent("update:installing", func(data interface{}) {
		a.emitEvent("update:installing", data)
	})

	a.updater.OnEvent("update:ready", func(data interface{}) {
		a.emitEvent("update:ready", data)
	})
}

// CheckForUpdates checks for available updates
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("updater not initialized")
	}

	a.logInfo("Checking for updates...")
	return a.updater.CheckForUpdates()
}

// DownloadUpdate downloads and installs an update
func (a *App) DownloadUpdate(version string) error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}

	a.logInfo(fmt.Sprintf("Downloading update: %s", version))
	return a.updater.DownloadAndInstall(version)
}

// GetUpdateStatus returns the current update status
func (a *App) GetUpdateStatus() map[string]interface{} {
	if a.updater == nil {
		return map[string]interface{}{
			"checking":        false,
			"downloading":     false,
			"installing":      false,
			"updateAvailable": false,
			"currentVersion":  updater.Version,
			"error":           "Updater not initialized",
		}
	}

	status := a.updater.GetStatus()
	return map[string]interface{}{
		"checking":         status.Checking,
		"downloading":      status.Downloading,
		"installing":       status.Installing,
		"updateAvailable":  status.UpdateAvailable,
		"currentVersion":   status.CurrentVersion,
		"latestVersion":    status.LatestVersion,
		"downloadProgress": status.DownloadProgress,
		"error":            status.Error,
		"lastChecked":      status.LastChecked,
	}
}

// SetUpdateChannel sets the update channel (stable or beta)
func (a *App) SetUpdateChannel(channel string) error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}

	var ch updater.UpdateChannel
	if channel == "beta" {
		ch = updater.ChannelBeta
	} else {
		ch = updater.ChannelStable
	}

	a.updater.SetChannel(ch)
	a.logInfo(fmt.Sprintf("Update channel set to: %s", channel))
	return nil
}

// SetAutoUpdateCheck enables or disables automatic update checking
func (a *App) SetAutoUpdateCheck(enabled bool) error {
	if a.updateChecker == nil {
		return fmt.Errorf("update checker not initialized")
	}

	a.updateChecker.SetEnabled(enabled)
	if enabled && !a.updateChecker.IsRunning() {
		a.updateChecker.Start()
	}
	a.logInfo(fmt.Sprintf("Auto-update checking: %v", enabled))
	return nil
}

// OpenReleaseURL opens the GitHub releases page in the default browser
func (a *App) OpenReleaseURL() error {
	url := fmt.Sprintf("https://github.com/%s/%s/releases", updater.GitHubOwner, updater.GitHubRepo)
	wailsRuntime.BrowserOpenURL(a.ctx, url)
	a.logInfo("Opened GitHub releases page")
	return nil
}

// RestartApplication restarts the application to apply updates
func (a *App) RestartApplication() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}

	a.logInfo("Restarting application to apply update...")

	// Call the updater's restart method
	if err := a.updater.RestartApplication(); err != nil {
		a.logError(fmt.Sprintf("Failed to restart application: %v", err))
		return err
	}

	// Give the restart script a moment to launch
	time.Sleep(500 * time.Millisecond)

	// Exit the current application
	wailsRuntime.Quit(a.ctx)

	return nil
}

// ===== EVENT HANDLING =====

// setupIntegrationEventHandlers sets up event handlers from integration to forward to frontend
func (a *App) setupIntegrationEventHandlers() {
	if a.integration == nil {
		return
	}

	// Forward integration events to Wails events
	a.integration.OnEvent("connected", func(data interface{}) {
		a.logInfo("WebSocket connected")
		a.emitEvent("integration:connected", data)
	})

	a.integration.OnEvent("disconnected", func(data interface{}) {
		a.logInfo("WebSocket disconnected")
		a.emitEvent("integration:disconnected", data)
	})

	a.integration.OnEvent("mapDetected", func(data interface{}) {
		if mapName, ok := data.(string); ok {
			a.logInfo(fmt.Sprintf("Map detected: %s", mapName))
			// Keep the map viewer window in sync
			if a.overlayServer != nil {
				a.overlayServer.SetCurrentMap(mapName)
			}
		}
		a.emitEvent("integration:mapDetected", data)
	})

	a.integration.OnEvent("screenshotProcessed", func(data interface{}) {
		if mapData, ok := data.(map[string]interface{}); ok {
			// Check if this was a successful position extraction
			if success, ok := mapData["success"].(bool); ok && success {
				if posData, ok := mapData["positionData"].(*position.PositionData); ok {
					a.logDebug(fmt.Sprintf("Screenshot processed: %.2f, %.2f, %.2f on %s",
						posData.X, posData.Y, posData.Z, posData.Map))

					// Forward position to centralized party if connected
					if err := a.SendPositionToParty(posData.Map, posData.X, posData.Y, posData.Z, posData.Rotation); err != nil {
						if centralPartyState.inParty {
							a.logDebug(fmt.Sprintf("Failed to send position to party: %v", err))
						}
					}
				}
			}
		}
		a.emitEvent("integration:screenshotProcessed", data)
	})

	a.integration.OnEvent("questStatusChanged", func(data interface{}) {
		a.logInfo("Quest status changed")
		a.emitEvent("integration:questStatusChanged", data)
	})

	a.integration.OnEvent("error", func(data interface{}) {
		if errMsg, ok := data.(string); ok {
			a.logError(errMsg)
		} else {
			a.logError(fmt.Sprintf("Integration error: %v", data))
		}
		a.emitEvent("integration:error", data)
	})

	// Party events
	a.integration.OnEvent("party:welcome", func(data interface{}) {
		a.logInfo("Joined party successfully")

		// Parse members from welcome message
		if msg, ok := data.(*party.WelcomeMessage); ok {
			a.stateMutex.Lock()
			a.partyMembers = make([]map[string]interface{}, 0)
			for _, m := range msg.Members {
				member := map[string]interface{}{
					"remoteId":    m.RemoteID,
					"displayName": m.DisplayName,
					"connected":   m.Connected,
					"currentMap":  "", // Initial state doesn't have map
					"lastUpdate":  time.Now().Format(time.RFC3339),
				}
				a.partyMembers = append(a.partyMembers, member)
			}
			a.stateMutex.Unlock()
		}

		a.emitEvent("party:welcome", data)
	})

	a.integration.OnEvent("party:memberJoined", func(data interface{}) {
		a.logInfo("Party member joined")

		// Add new member to tracking list
		if msg, ok := data.(*party.MemberJoinedMessage); ok {
			a.stateMutex.Lock()
			// Check if already exists
			exists := false
			for _, m := range a.partyMembers {
				if id, ok := m["remoteId"].(string); ok && id == msg.Member.RemoteID {
					exists = true
					break
				}
			}

			if !exists {
				member := map[string]interface{}{
					"remoteId":    msg.Member.RemoteID,
					"displayName": msg.Member.DisplayName,
					"connected":   true,
					"currentMap":  "",
					"lastUpdate":  time.Now().Format(time.RFC3339),
				}
				a.partyMembers = append(a.partyMembers, member)
			}
			a.stateMutex.Unlock()
		}

		a.emitEvent("party:memberJoined", data)
	})

	a.integration.OnEvent("party:memberLeft", func(data interface{}) {
		a.logInfo("Party member left")

		// Remove member from tracking list
		if msg, ok := data.(*party.MemberLeftMessage); ok {
			a.stateMutex.Lock()
			newMembers := make([]map[string]interface{}, 0)
			for _, m := range a.partyMembers {
				if id, ok := m["remoteId"].(string); ok && id != msg.RemoteID {
					newMembers = append(newMembers, m)
				}
			}
			a.partyMembers = newMembers
			a.stateMutex.Unlock()
		}

		a.emitEvent("party:memberLeft", data)
	})

	a.integration.OnEvent("party:positionReceived", func(data interface{}) {
		// Update member position/map in tracking list
		if msg, ok := data.(*party.PositionUpdateMessage); ok {
			a.stateMutex.Lock()
			for i, m := range a.partyMembers {
				if id, ok := m["remoteId"].(string); ok && id == msg.RemoteID {
					// Update map and position
					a.partyMembers[i]["currentMap"] = msg.Map
					a.partyMembers[i]["lastUpdate"] = time.Now().Format(time.RFC3339)
					a.partyMembers[i]["lastPosition"] = map[string]interface{}{
						"x":        msg.Position.X,
						"y":        msg.Position.Y,
						"z":        msg.Position.Z,
						"rotation": msg.Position.Rotation,
					}
					break
				}
			}
			a.stateMutex.Unlock()
		}

		a.emitEvent("party:positionReceived", data)
	})

	a.integration.OnEvent("party:error", func(data interface{}) {
		a.logError("Party error occurred")
		a.emitEvent("party:error", data)
	})

	a.integration.OnEvent("party:disconnected", func(data interface{}) {
		a.logWarning("Disconnected from party")
		a.emitEvent("party:disconnected", data)
	})
}

// emitEvent emits an event to the frontend
func (a *App) emitEvent(eventName string, data interface{}) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, eventName, data)
}

// ===== LOGGING HELPERS =====

func (a *App) logInfo(message string) {
	a.logger.Add("info", message)
	log.Println("[INFO]", message)
	a.emitEvent("log:added", LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "info",
		Message:   message,
	})
}

func (a *App) logWarning(message string) {
	a.logger.Add("warning", message)
	log.Println("[WARNING]", message)
	a.emitEvent("log:added", LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "warning",
		Message:   message,
	})
}

func (a *App) logError(message string) {
	a.logger.Add("error", message)
	log.Println("[ERROR]", message)
	a.emitEvent("log:added", LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "error",
		Message:   message,
	})
}

func (a *App) logDebug(message string) {
	// Only log debug messages if debug logging is enabled
	if a.config != nil && a.config.DebugLogging {
		a.logger.Add("debug", message)
		log.Println("[DEBUG]", message)
		a.emitEvent("log:added", LogEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "debug",
			Message:   message,
		})
	}
}

// ===== LOGGER INTERFACE IMPLEMENTATION =====
// These methods implement the logger.Logger interface so App can be passed to backend services

func (a *App) Debug(message string) {
	a.logDebug(message)
}

func (a *App) Info(message string) {
	a.logInfo(message)
}

func (a *App) Warning(message string) {
	a.logWarning(message)
}

func (a *App) Error(message string) {
	a.logError(message)
}
