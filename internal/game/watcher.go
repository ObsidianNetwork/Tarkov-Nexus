package game

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/maps"
)

// RaidInfo represents current raid information
type RaidInfo struct {
	Map           string    `json:"map"`
	NormalizedMap string    `json:"normalizedMap"`
	RaidID        string    `json:"raidId"`
	Online        bool      `json:"online"`
	ProfileType   string    `json:"profileType"`
	StartedTime   *time.Time `json:"startedTime"`
	EndedTime     *time.Time `json:"endedTime"`
	MapLoadTime   float64   `json:"mapLoadTime"`
	QueueTime     float64   `json:"queueTime"`
}

// Profile represents player profile information
type Profile struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Watcher monitors Escape from Tarkov log files for game events
type Watcher struct {
	logsPath        string
	configLogsPath  string // Configured logs path from config
	mapResolver     *maps.Resolver
	currentProfile  Profile
	raidInfo        RaidInfo
	pollingActive   map[string]bool
	filePositions   map[string]int64
	initialLogsRead bool
	ctx             context.Context
	cancel          context.CancelFunc
	eventHandlers   map[string][]func(interface{})
	handlerMutex    sync.RWMutex
	watcherMutex    sync.Mutex
	logger          logger.Logger
}

// NewWatcher creates a new game log watcher
func NewWatcher(log logger.Logger) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		mapResolver:     maps.NewResolver(),
		currentProfile:  Profile{Type: "Regular"},
		raidInfo:        RaidInfo{ProfileType: "Regular"},
		pollingActive:   make(map[string]bool),
		filePositions:   make(map[string]int64),
		ctx:             ctx,
		cancel:          cancel,
		eventHandlers:   make(map[string][]func(interface{})),
		logger:          log,
	}
}

// SetLogsPath sets a specific logs path to use
func (gw *Watcher) SetLogsPath(path string) {
	gw.configLogsPath = path
}

// Start begins monitoring game logs
func (gw *Watcher) Start() error {
	logsPath, err := gw.getDefaultLogsFolder()
	if err != nil {
		return fmt.Errorf("failed to find logs folder: %w", err)
	}

	gw.logsPath = logsPath

	latestLogFolder, err := gw.getLatestLogFolder()
	if err != nil {
		return fmt.Errorf("failed to find latest log folder: %w", err)
	}

	// If no log folder found, skip log monitoring but don't fail
	if latestLogFolder == "" {
		gw.logger.Info(fmt.Sprintf("📋 Log monitoring disabled - logs directory not found"))
		gw.logger.Info(fmt.Sprintf("    Screenshot position tracking will still work"))
		return nil
	}

	gw.logger.Info(fmt.Sprintf("📋 Watching logs in: %s", latestLogFolder))

	return gw.watchLogsFolder(latestLogFolder)
}

// Stop stops all log monitoring
func (gw *Watcher) Stop() {
	gw.watcherMutex.Lock()
	defer gw.watcherMutex.Unlock()

	// Cancel context to stop all polling goroutines
	gw.cancel()
	
	// Clear polling state
	gw.pollingActive = make(map[string]bool)
	gw.filePositions = make(map[string]int64)

	gw.logger.Info(fmt.Sprintf("📋 Stopped all log monitoring"))
}

// getDefaultLogsFolder finds the Escape from Tarkov logs directory
func (gw *Watcher) getDefaultLogsFolder() (string, error) {
	// Check configured path first
	if gw.configLogsPath != "" {
		if _, err := os.Stat(gw.configLogsPath); err == nil {
			gw.logger.Debug(fmt.Sprintf("📋 Using configured logs directory: %s", gw.configLogsPath))
			return gw.configLogsPath, nil
		}
		gw.logger.Warning(fmt.Sprintf("⚠️  Configured logs directory not found: %s", gw.configLogsPath))
	}

	// Check environment variable next
	if logsPath := os.Getenv("TARKOV_LOGS_PATH"); logsPath != "" {
		if _, err := os.Stat(logsPath); err == nil {
			return logsPath, nil
		}
	}

	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("EFT is only supported on Windows")
	}

	// Try Windows Registry detection first (like JavaScript implementation)
	if registryPath := gw.getEFTPathFromRegistry(); registryPath != "" {
		logsPath := filepath.Join(registryPath, "Logs")
		if _, err := os.Stat(logsPath); err == nil {
			gw.logger.Debug(fmt.Sprintf("📋 Found EFT logs via Windows Registry: %s", logsPath))
			return logsPath, nil
		}
		gw.logger.Debug(fmt.Sprintf("📋 Registry found EFT at %s but no Logs folder", registryPath))
	}

	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Try common EFT installation paths
	defaultPaths := []string{
		// Most common paths
		`C:\Battlestate Games\EscapeFromTarkov\Logs`,
		`C:\Program Files\Battlestate Games\EscapeFromTarkov\Logs`,
		`C:\Program Files (x86)\Battlestate Games\EscapeFromTarkov\Logs`,
		
		// User-specific installations
		filepath.Join(homeDir, "AppData", "Local", "Battlestate Games", "EscapeFromTarkov", "Logs"),
		filepath.Join(homeDir, "Documents", "Battlestate Games", "EscapeFromTarkov", "Logs"),
		
		// Steam installations
		`C:\Program Files (x86)\Steam\steamapps\common\EscapeFromTarkov\Logs`,
		filepath.Join(homeDir, "Steam", "steamapps", "common", "EscapeFromTarkov", "Logs"),
		
		// Alternative drive installations
		`D:\Battlestate Games\EscapeFromTarkov\Logs`,
		`D:\Program Files\Battlestate Games\EscapeFromTarkov\Logs`,
		`E:\Battlestate Games\EscapeFromTarkov\Logs`,
		`E:\Program Files\Battlestate Games\EscapeFromTarkov\Logs`,
	}

	for _, logPath := range defaultPaths {
		if _, err := os.Stat(logPath); err == nil {
			gw.logger.Debug(fmt.Sprintf("📋 Found EFT logs directory: %s", logPath))
			return logPath, nil
		}
	}

	// If we can't find logs, warn but don't fail - maybe user is only using screenshots
	gw.logger.Warning(fmt.Sprintf("⚠️  EFT logs directory not found. Game events will not be detected."))
	gw.logger.Warning(fmt.Sprintf("    If you want map detection from logs, set TARKOV_LOGS_PATH environment variable"))
	gw.logger.Warning(fmt.Sprintf("    Screenshot position tracking will still work without logs"))
	
	// Return a dummy path that won't cause the application to fail
	return filepath.Join(homeDir, "Documents", "Battlestate Games", "EscapeFromTarkov", "Logs"), nil
}

// getLatestLogFolder finds the most recent log folder
func (gw *Watcher) getLatestLogFolder() (string, error) {
	if _, err := os.Stat(gw.logsPath); os.IsNotExist(err) {
		gw.logger.Warning(fmt.Sprintf("⚠️  Logs directory does not exist: %s", gw.logsPath))
		gw.logger.Warning(fmt.Sprintf("    Map detection from logs will be disabled"))
		return "", nil // Return nil error to allow continuation
	}

	entries, err := os.ReadDir(gw.logsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read logs directory: %w", err)
	}

	type logFolder struct {
		name string
		time time.Time
	}

	var logFolders []logFolder
	logFolderRegex := regexp.MustCompile(`log_(\d{4})\.(\d{2})\.(\d{2})_(\d{1,2})-(\d{2})-(\d{2})`)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		matches := logFolderRegex.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		// Parse time components
		year := parseInt(matches[1])
		month := parseInt(matches[2])
		day := parseInt(matches[3])
		hour := parseInt(matches[4])
		minute := parseInt(matches[5])
		second := parseInt(matches[6])

		folderTime := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)

		logFolders = append(logFolders, logFolder{
			name: entry.Name(),
			time: folderTime,
		})
	}

	if len(logFolders) == 0 {
		return "", fmt.Errorf("no log folders found")
	}

	// Sort by time, most recent first
	sort.Slice(logFolders, func(i, j int) bool {
		return logFolders[i].time.After(logFolders[j].time)
	})

	// Debug: show all found log folders
	gw.logger.Debug(fmt.Sprintf("📋 Found %d log folders:", len(logFolders)))
	for i, folder := range logFolders {
		gw.logger.Debug(fmt.Sprintf("📋   [%d] %s -> %s", i, folder.name, folder.time.Format("2006-01-02 15:04:05")))
	}
	gw.logger.Debug(fmt.Sprintf("📋 Selected most recent: %s", logFolders[0].name))

	return filepath.Join(gw.logsPath, logFolders[0].name), nil
}

// watchLogsFolder starts monitoring all log files in the given folder
func (gw *Watcher) watchLogsFolder(folderPath string) error {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return fmt.Errorf("failed to read log folder: %w", err)
	}

	logFiles := make([]struct {
		file string
		type_ string
	}, 0)

	// Debug: List all files in the log folder
	gw.logger.Debug(fmt.Sprintf("📋 Scanning log folder for files..."))
	allFiles := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			allFiles = append(allFiles, entry.Name())
		}
	}
	if len(allFiles) > 0 {
		gw.logger.Debug(fmt.Sprintf("📋 Found %d files in log folder:", len(allFiles)))
		for i, file := range allFiles {
			if i < 10 { // Show first 10 files to avoid spam
				gw.logger.Debug(fmt.Sprintf("📋   - %s", file))
			}
		}
		if len(allFiles) > 10 {
			gw.logger.Debug(fmt.Sprintf("📋   ... and %d more files", len(allFiles)-10))
		}
	} else {
		gw.logger.Debug(fmt.Sprintf("📋 No files found in log folder"))
	}

	// Find application log file with flexible pattern matching
	// Tarkov v1.0+ uses application_XXX.log format (e.g., application_000.log)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		filenameLower := strings.ToLower(filename)

		// Check for application log (case-insensitive, multiple patterns)
		// Matches: application.log, application_000.log, etc.
		if strings.Contains(filenameLower, "application") && strings.HasSuffix(filenameLower, ".log") {
			logFiles = append(logFiles, struct {
				file string
				type_ string
			}{filename, "application"})
			gw.logger.Debug(fmt.Sprintf("📋 Found application log: %s", filename))
		} else if strings.Contains(filenameLower, "notifications") && strings.HasSuffix(filenameLower, ".log") {
			logFiles = append(logFiles, struct {
				file string
				type_ string
			}{filename, "notifications"})
			gw.logger.Debug(fmt.Sprintf("📋 Found notifications log: %s", filename))
		} else if strings.Contains(filenameLower, "app") && strings.HasSuffix(filenameLower, ".log") && !strings.Contains(filenameLower, "application") {
			// Fallback: look for "app*.log" files (new format might use "app.log")
			logFiles = append(logFiles, struct {
				file string
				type_ string
			}{filename, "application"})
			gw.logger.Debug(fmt.Sprintf("📋 Found application log (alternative pattern): %s", filename))
		}
	}

	if len(logFiles) == 0 {
		gw.logger.Warning(fmt.Sprintf("⚠️  No log files found in %s", folderPath))
		gw.logger.Warning(fmt.Sprintf("    Log monitoring will be disabled, but screenshot tracking will still work"))
		gw.logger.Warning(fmt.Sprintf("    If this is a new Tarkov version, the log file format may have changed"))
		gw.logger.Warning(fmt.Sprintf("    Please report this issue with a list of files in the log folder"))
		// Don't return error - allow program to continue without log monitoring
		return nil
	}

	for _, logFile := range logFiles {
		logPath := filepath.Join(folderPath, logFile.file)
		gw.logger.Debug(fmt.Sprintf("📋 Starting watcher for %s log: %s", logFile.type_, logPath))
		if err := gw.startLogFileWatcher(logPath, logFile.type_); err != nil {
			gw.logger.Error(fmt.Sprintf("Failed to start watcher for %s: %v", logPath, err))
		}
	}

	return nil
}

// startLogFileWatcher starts monitoring a specific log file using polling (like JavaScript)
func (gw *Watcher) startLogFileWatcher(logPath, logType string) error {
	gw.logger.Debug(fmt.Sprintf("📋 Starting %s monitor at %s", logType, logPath))

	// For non-application logs, start from end of file for live monitoring
	initialPosition := int64(0)
	if logType != "application" {
		if stat, err := os.Stat(logPath); err == nil {
			initialPosition = stat.Size()
			gw.logger.Debug(fmt.Sprintf("📋 %s log: starting from end (position %d) for live monitoring", logType, initialPosition))
		}
	}

	// Read existing content first (only for application logs)
	if logType == "application" {
		if err := gw.processExistingLogContent(logPath, logType); err != nil {
			gw.logger.Error(fmt.Sprintf("Failed to process existing log content: %v", err))
		}
	}

	// Set initial file position (thread-safe)
	gw.watcherMutex.Lock()
	gw.filePositions[logPath] = initialPosition
	gw.pollingActive[logPath] = true
	gw.watcherMutex.Unlock()

	// Start polling-based monitoring (like JavaScript does every 5 seconds)
	go gw.pollLogFile(logPath, logType)

	return nil
}

// processExistingLogContent reads and processes existing log content
func (gw *Watcher) processExistingLogContent(logPath, logType string) error {
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// Store current position for future reads (thread-safe)
	gw.watcherMutex.Lock()
	gw.filePositions[logPath] = stat.Size()
	gw.watcherMutex.Unlock()

	// Process the content
	scanner := bufio.NewScanner(file)
	var content strings.Builder

	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	if content.Len() > 0 {
		// Check if this is an old log file (more than 2 hours old)
		logAge := time.Since(stat.ModTime())
		isOldLog := logAge > 2*time.Hour

		if isOldLog {
			gw.logger.Warning(fmt.Sprintf("📋 Processing old log file (age: %.1f hours) - map state may be outdated", logAge.Hours()))
		}

		gw.handleNewLogData(logPath, logType, content.String(), true)

		// If this is an old log and we detected a raid, warn that it might be outdated
		if isOldLog && gw.raidInfo.Map != "" {
			gw.logger.Warning(fmt.Sprintf("⚠️  Detected old raid state from logs: %s", gw.raidInfo.Map))
			gw.logger.Warning(fmt.Sprintf("    This may not reflect current game state. Start a new raid to update."))
		}
	}

	return scanner.Err()
}

// pollLogFile polls a log file for new data every 5 seconds (like JavaScript implementation)
func (gw *Watcher) pollLogFile(logPath, logType string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	gw.logger.Debug(fmt.Sprintf("📋 Started polling %s log every 5 seconds", logType))

	for {
		select {
		case <-gw.ctx.Done():
			gw.logger.Debug(fmt.Sprintf("📋 Stopped polling %s log", logType))
			return

		case <-ticker.C:
			// Check for new data (like JavaScript checkForNewData)
			if err := gw.checkForNewLogData(logPath, logType); err != nil {
				gw.logger.Error(fmt.Sprintf("Error checking %s log for new data: %v", logType, err))
			}
		}
	}
}

// checkForNewLogData checks for new data in log file (like JavaScript checkForNewData)
func (gw *Watcher) checkForNewLogData(logPath, logType string) error {
	stat, err := os.Stat(logPath)
	if err != nil {
		return err
	}

	currentSize := stat.Size()
	
	// Thread-safe access to filePositions
	gw.watcherMutex.Lock()
	lastPosition := gw.filePositions[logPath]
	gw.watcherMutex.Unlock()

	// Check if file has new data (like JavaScript: fileSize > this.fileBytesRead)
	if currentSize <= lastPosition {
		return nil // No new data
	}

	// Read only the new data from last position
	newData, err := gw.readIncrementalData(logPath, lastPosition, currentSize)
	if err != nil {
		return err
	}

	// Thread-safe update of position
	gw.watcherMutex.Lock()
	gw.filePositions[logPath] = currentSize
	gw.watcherMutex.Unlock()

	if len(newData) > 0 {
		gw.logger.Debug(fmt.Sprintf("📋 %s log: read %d new bytes (pos %d -> %d)", logType, len(newData), lastPosition, currentSize))
		gw.handleNewLogData(logPath, logType, string(newData), false)
	}

	return nil
}

// readIncrementalData reads only new data from file (like JavaScript readNewData)
func (gw *Watcher) readIncrementalData(logPath string, fromPos, toPos int64) ([]byte, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Seek to last read position
	if _, err := file.Seek(fromPos, 0); err != nil {
		return nil, err
	}

	// Read only the new data
	newDataSize := toPos - fromPos
	buffer := make([]byte, newDataSize)

	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

// handleNewLogData processes new log data
func (gw *Watcher) handleNewLogData(logPath, logType, data string, initialRead bool) {
	gw.logger.Debug(fmt.Sprintf("📋 Received %s log data (%d chars, initial: %t)", logType, len(data), initialRead))

	gw.emit("newLogData", map[string]interface{}{
		"path":        logPath,
		"type":        logType,
		"data":        data,
		"initialRead": initialRead,
	})

	gw.parseLogData(data, initialRead)
}

// parseLogData parses log data for game events
func (gw *Watcher) parseLogData(data string, initialRead bool) {
	// First, check for quest notifications using multiline JSON pattern (like reprocessing)
	gw.processQuestNotifications(data, initialRead)

	// Then process regular log events line by line
	// Updated regex to support both old format (with timezone) and new format (without timezone)
	// Old: 2025-07-28 18:17:05.049 +10:00|...
	// New: 2025-11-19 21:03:55.686|...
	logPattern := regexp.MustCompile(`(?m)^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}\.\d{3})(?: [+-]\d{2}:\d{2})?\|(.+)$`)

	matches := logPattern.FindAllStringSubmatch(data, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		eventLine := match[3]
		eventTimeStr := match[1] + " " + match[2]

		eventTime, err := time.Parse("2006-01-02 15:04:05.000", eventTimeStr)
		if err != nil {
			continue
		}

		gw.processLogEvent(eventLine, eventTime, initialRead)
	}
}

// processQuestNotifications handles quest notifications with multiline JSON (like reprocessing function)
func (gw *Watcher) processQuestNotifications(data string, initialRead bool) {
	// Updated regex to support both old format (with timezone) and new format (without timezone)
	// Old: 2025-07-28 18:17:05.049 +10:00|...
	// New: 2025-11-19 21:03:55.686|...
	logPattern := `(?m)(?P<date>^\d{4}-\d{2}-\d{2}) (?P<time>\d{2}:\d{2}:\d{2}\.\d{3})(?: [+-]\d{2}:\d{2})?\|(?P<message>.+$)\s*(?P<json>^{[\s\S]+?^})?`

	re := regexp.MustCompile(logPattern)
	matches := re.FindAllStringSubmatch(data, -1)
	
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}
		
		eventLine := match[3] // message part
		jsonString := match[4] // json part
		
		// Only process ChatMessageReceived notifications
		if !strings.Contains(eventLine, "Got notification | ChatMessageReceived") {
			continue
		}
		
		// Skip if no JSON part
		if jsonString == "" {
			continue
		}
		
		// Combine the log line with the JSON for processing
		fullNotification := eventLine + "\n" + jsonString
		
		if IsQuestNotification(fullNotification) {
			questEvent, err := ParseQuestNotification(fullNotification)
			if err != nil {
				gw.logger.Warning(fmt.Sprintf("⚠️  Failed to parse quest notification in live monitoring: %v", err))
				continue
			}

			// Skip quest processing during initial read unless we want to catch up on recent quests
			if initialRead {
				gw.logger.Debug(fmt.Sprintf("📋 Skipping quest from initial read: %s", questEvent.Summary()))
				continue
			}

			gw.logger.Info(fmt.Sprintf("📋 Live quest detected: %s", questEvent.Summary()))
			gw.emit("questStatusChanged", questEvent)
		}
	}
}

// processLogEvent processes a single log event
func (gw *Watcher) processLogEvent(eventLine string, eventTime time.Time, initialRead bool) {
	// Debug: Log interesting events
	if strings.Contains(eventLine, "application|") &&
		(strings.Contains(eventLine, "LocationLoaded") ||
			strings.Contains(eventLine, "MatchingCompleted") ||
			strings.Contains(eventLine, "TRACE-NetworkGameCreate") ||
			strings.Contains(eventLine, "GameStarting") ||
			strings.Contains(eventLine, "GameStarted") ||
			strings.Contains(eventLine, "SelectProfile") ||
			strings.Contains(eventLine, "matching")) {

		if strings.Contains(eventLine, "TRACE-NetworkGameCreate") {
			gw.logger.Debug(fmt.Sprintf("📋 TRACE-FULL: %s", eventLine))
		} else {
			eventPreview := eventLine
			if len(eventPreview) > 150 {
				eventPreview = eventPreview[:150] + "..."
			}
			gw.logger.Debug(fmt.Sprintf("📋 DEBUG: %s", eventPreview))
		}
	}

	// Profile selection
	if strings.Contains(eventLine, "SelectProfile ProfileId:") {
		profileRegex := regexp.MustCompile(`SelectProfile ProfileId:(\w+)`)
		matches := profileRegex.FindStringSubmatch(eventLine)
		if len(matches) > 1 {
			gw.currentProfile.ID = matches[1]
			
			// During initial read, SelectProfile indicates the current user is back in menu
			// This means any previous raid from these logs has ended
			if initialRead {
				gw.logger.Debug(fmt.Sprintf("📋 SelectProfile detected during initial read - clearing any old raid state"))
				// Reset raid info to indicate no active raid
				gw.raidInfo = RaidInfo{
					ProfileType: gw.currentProfile.Type,
				}
			} else {
				// During live monitoring, SelectProfile means raid ended
				if gw.raidInfo.StartedTime != nil && gw.raidInfo.EndedTime == nil {
					gw.raidInfo.EndedTime = &eventTime
					gw.logger.Info(fmt.Sprintf("🚪 Raid ended (detected via SelectProfile)"))
					gw.emit("raidEnded", map[string]interface{}{
						"raidInfo": gw.raidInfo,
						"profile":  gw.currentProfile,
					})
				} else {
					gw.emit("profileChanged", gw.currentProfile)
				}
			}
		}
		return
	}

	// Session mode (profile type)
	if strings.Contains(eventLine, "Session mode: ") {
		modeRegex := regexp.MustCompile(`Session mode: (\w+)`)
		matches := modeRegex.FindStringSubmatch(eventLine)
		if len(matches) > 1 {
			gw.currentProfile.Type = matches[1]
			gw.raidInfo.ProfileType = gw.currentProfile.Type
		}
		return
	}

	// Skip initial read events for most processing (like JavaScript implementation)
	// During initial read, only process profile selection and latest map state
	if initialRead {
		// Only allow profile selection during initial read
		if !strings.Contains(eventLine, "SelectProfile") {
			return
		}
	}

	// Scene preset path - can be used as fallback for map detection
	if strings.Contains(eventLine, "application|scene preset path:maps/") {
		sceneRegex := regexp.MustCompile(`scene preset path:maps/([^_]+)(?:_([^.]+))?_preset\.bundle`)
		matches := sceneRegex.FindStringSubmatch(eventLine)
		if len(matches) > 1 {
			baseMap := matches[1]
			variant := ""
			if len(matches) > 2 && matches[2] != "" {
				variant = matches[2]
			}
			
			// Construct map name (e.g., "factory_day", "woods")
			mapName := baseMap
			if variant != "" {
				mapName = baseMap + "_" + variant
			}
			
			normalizedMapName := gw.mapResolver.GetNormalizedName(mapName)
			displayName := gw.mapResolver.GetDisplayName(mapName)
			
			if normalizedMapName != "" {
				// Store as fallback map info
				gw.raidInfo.Map = mapName
				gw.raidInfo.NormalizedMap = normalizedMapName
				gw.raidInfo.Online = false // Scene preset usually indicates offline

				gw.logger.Info(fmt.Sprintf("🗺️  Fallback map detected from scene preset: %s -> %s (%s) [Offline]", mapName, normalizedMapName, displayName))

				// Emit map loaded event
				gw.emit("mapLoaded", map[string]interface{}{
					"raidInfo": gw.raidInfo,
					"profile":  gw.currentProfile,
				})
			} else {
				gw.logger.Warning(fmt.Sprintf("⚠️  Unknown map in scene preset: %s", mapName))
			}
		}
		return
	}

	// LocationLoaded - The map has been loaded and the game is searching for a match
	if strings.Contains(eventLine, "application|LocationLoaded") {
		// Don't reset map info if we already have it from scene preset
		if gw.raidInfo.Map == "" {
			gw.raidInfo = RaidInfo{
				ProfileType: gw.currentProfile.Type,
			}
		} else {
			// Keep existing map info but reset other raid state
			existingMap := gw.raidInfo.Map
			existingNormalizedMap := gw.raidInfo.NormalizedMap
			existingOnline := gw.raidInfo.Online
			
			gw.raidInfo = RaidInfo{
				Map:           existingMap,
				NormalizedMap: existingNormalizedMap,
				Online:        existingOnline,
				ProfileType:   gw.currentProfile.Type,
			}
		}

		gw.logger.Info(fmt.Sprintf("📋 LocationLoaded - matching started"))
		gw.emit("matchingStarted", map[string]interface{}{
			"raidInfo": gw.raidInfo,
			"profile":  gw.currentProfile,
		})
		return
	}

	// TRACE-NetworkGameCreate - Map info is available here
	if strings.Contains(eventLine, "application|TRACE-NetworkGameCreate") {
		mapRegex := regexp.MustCompile(`Location: ([^,\s]+)`)
		raidIDRegex := regexp.MustCompile(`shortId: ([A-Z0-9]{6})`)

		mapMatches := mapRegex.FindStringSubmatch(eventLine)
		raidIDMatches := raidIDRegex.FindStringSubmatch(eventLine)

		if len(mapMatches) > 1 {
			internalMapName := mapMatches[1]
			normalizedMapName := gw.mapResolver.GetNormalizedName(internalMapName)
			displayName := gw.mapResolver.GetDisplayName(internalMapName)

			if normalizedMapName != "" {
				gw.raidInfo.Map = internalMapName
				gw.raidInfo.NormalizedMap = normalizedMapName
				gw.raidInfo.Online = strings.Contains(eventLine, "RaidMode: Online")

				if len(raidIDMatches) > 1 {
					gw.raidInfo.RaidID = raidIDMatches[1]
				}

				onlineStatus := "Offline"
				if gw.raidInfo.Online {
					onlineStatus = "Online"
				}

				gw.logger.Info(fmt.Sprintf("🗺️  Map detected: %s -> %s (%s) [%s]", internalMapName, normalizedMapName, displayName, onlineStatus))
				gw.emit("mapLoaded", map[string]interface{}{
					"raidInfo": gw.raidInfo,
					"profile":  gw.currentProfile,
				})
			} else {
				gw.logger.Warning(fmt.Sprintf("⚠️  Unknown map detected: %s - adding to map resolver needed", internalMapName))
			}
		}
		return
	}

	// GameStarted - Raid begins
	if strings.Contains(eventLine, "application|GameStarted") {
		if gw.raidInfo.StartedTime == nil {
			gw.raidInfo.StartedTime = &eventTime
		}
		gw.logger.Info(fmt.Sprintf("🎮 GameStarted - raid active on %s", gw.raidInfo.Map))
		gw.emit("raidStarted", map[string]interface{}{
			"raidInfo": gw.raidInfo,
			"profile":  gw.currentProfile,
		})
		return
	}

	// Quest notifications are now handled by processQuestNotifications() with multiline JSON support

	// UserMatchOver notification - Raid ended
	if strings.Contains(eventLine, "Got notification | UserMatchOver") {
		gw.raidInfo.EndedTime = &eventTime
		gw.logger.Info(fmt.Sprintf("🚪 Raid exited via UserMatchOver"))
		gw.emit("raidExited", gw.raidInfo)

		// Reset raid info
		gw.raidInfo = RaidInfo{
			ProfileType: gw.currentProfile.Type,
		}
		return
	}
}

// OnEvent registers an event handler
func (gw *Watcher) OnEvent(event string, handler func(interface{})) {
	gw.handlerMutex.Lock()
	defer gw.handlerMutex.Unlock()

	gw.eventHandlers[event] = append(gw.eventHandlers[event], handler)
}

// emit triggers event handlers
func (gw *Watcher) emit(event string, data interface{}) {
	gw.handlerMutex.RLock()
	defer gw.handlerMutex.RUnlock()

	if handlers, exists := gw.eventHandlers[event]; exists {
		for _, handler := range handlers {
			go func(h func(interface{}), ev string) {
				defer func() {
					if r := recover(); r != nil {
						gw.logger.Error(fmt.Sprintf("Panic in game watcher event handler [%s]: %v", ev, r))
					}
				}()
				h(data)
			}(handler, event)
		}
	}
}

// GetCurrentMap returns the current map
func (gw *Watcher) GetCurrentMap() string {
	return gw.raidInfo.Map
}

// GetRaidInfo returns a copy of the current raid info
func (gw *Watcher) GetRaidInfo() RaidInfo {
	return gw.raidInfo
}

// getEFTPathFromRegistry attempts to find EFT installation path from Windows Registry
func (gw *Watcher) getEFTPathFromRegistry() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	// Try the registry key that JavaScript uses
	registryKey := `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\EscapeFromTarkov`

	gw.logger.Debug(fmt.Sprintf("📋 Checking Windows Registry for EFT installation..."))

	cmd := exec.Command("reg", "query", registryKey, "/v", "InstallLocation")
	output, err := cmd.Output()
	if err != nil {
		gw.logger.Debug(fmt.Sprintf("📋 Registry query failed: %v", err))
		return ""
	}

	// Parse the registry output
	// Expected format: "    InstallLocation    REG_SZ    C:\Path\To\EFT"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "InstallLocation") && strings.Contains(line, "REG_SZ") {
			// Extract path from the line
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				// Find the path part (everything after REG_SZ)
				regSzIndex := -1
				for i, part := range parts {
					if part == "REG_SZ" {
						regSzIndex = i
						break
					}
				}
				if regSzIndex >= 0 && regSzIndex+1 < len(parts) {
					installPath := strings.Join(parts[regSzIndex+1:], " ")
					gw.logger.Debug(fmt.Sprintf("📋 Registry found EFT installation: %s", installPath))
					return installPath
				}
			}
		}
	}

	gw.logger.Debug(fmt.Sprintf("📋 Registry query succeeded but could not parse InstallLocation"))
	return ""
}

// Helper function to parse integer from string
func parseInt(s string) int {
	var result int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			result = result*10 + int(r-'0')
		}
	}
	return result
}