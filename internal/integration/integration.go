package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/config"
	"tarkov-screenshot-analyzer/internal/game"
	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/monitor"
	"tarkov-screenshot-analyzer/internal/overlay"
	"tarkov-screenshot-analyzer/internal/party"
	"tarkov-screenshot-analyzer/internal/position"
	"tarkov-screenshot-analyzer/internal/tracker"
	"tarkov-screenshot-analyzer/internal/websocket"
)

// Integration orchestrates all components of the tarkov.dev integration
type Integration struct {
	config            *config.Config
	logger            logger.Logger
	wsClient          *websocket.Client
	partyClient       *party.Client
	overlayServer     *overlay.Server
	screenshotMonitor *monitor.ScreenshotMonitor
	positionDetector  *position.Detector
	gameWatcher       *game.Watcher
	questTracker      *tracker.QuestTracker
	mapResolver       *game.MapResolver

	// State management
	lastPosition         *position.PositionData
	lastPositionUpdate   time.Time
	currentMap           string
	currentNormalizedMap string
	manualMapOverride    string // User-selected map when logs unavailable
	isInitialized        bool

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Event handlers
	eventHandlers map[string][]func(interface{})
	handlerMutex  sync.RWMutex

	// Synchronization
	stateMutex sync.RWMutex
}

// New creates a new tarkov.dev integration
func New(cfg *config.Config, log logger.Logger) *Integration {
	ctx, cancel := context.WithCancel(context.Background())

	return &Integration{
		config:           cfg,
		logger:           log,
		ctx:              ctx,
		cancel:           cancel,
		eventHandlers:    make(map[string][]func(interface{})),
		positionDetector: position.NewDetector(),
		mapResolver:      game.NewMapResolver(),
	}
}

// SetOverlayServer sets the overlay server instance
func (i *Integration) SetOverlayServer(server *overlay.Server) {
	i.overlayServer = server
}

// Initialize sets up all components and starts monitoring
func (i *Integration) Initialize() error {
	if i.isInitialized {
		i.logger.Debug("Integration already initialized")
		return nil
	}

	i.logger.Info("🚀 Initializing Tarkov.dev integration...")
	i.logger.Info(fmt.Sprintf("📁 Screenshot directory: %s", i.config.ScreenshotDir))

	// Safely display Remote ID (show first 8 chars or less if shorter)
	remoteIDDisplay := i.config.RemoteID
	if len(remoteIDDisplay) > 8 {
		remoteIDDisplay = remoteIDDisplay[:8] + "..."
	} else {
		remoteIDDisplay = remoteIDDisplay + "..."
	}
	i.logger.Info(fmt.Sprintf("🔗 Remote ID: %s", remoteIDDisplay))

	// Initialize WebSocket client
	i.wsClient = websocket.NewClient(
		i.config.RemoteID,
		time.Duration(i.config.ReconnectOptions.ReconnectIntervalMs)*time.Millisecond,
		i.config.ReconnectOptions.MaxReconnectAttempts,
		i.logger,
	)

	// Initialize screenshot monitor
	i.screenshotMonitor = monitor.NewScreenshotMonitor(
		i.config.ScreenshotDir,
		time.Duration(i.config.MonitorOptions.DebounceTimeMs)*time.Millisecond,
		i.logger,
	)

	// Initialize game watcher
	i.gameWatcher = game.NewWatcher(i.logger)

	// Set logs path if configured
	if i.config.LogsDir != "" {
		i.gameWatcher.SetLogsPath(i.config.LogsDir)
	}

	// Initialize quest tracker
	baseURL, token, enabled := i.config.GetTarkovTrackerConfig()
	i.questTracker = tracker.NewQuestTracker(tracker.Config{
		BaseURL: baseURL,
		Token:   token,
	}, enabled, i.logger)

	// Setup event handlers
	i.setupEventHandlers()

	// Start game log monitoring
	if err := i.gameWatcher.Start(); err != nil {
		return fmt.Errorf("failed to start game watcher: %w", err)
	}

	// Initialize quest tracking if enabled
	if i.questTracker.IsEnabled() {
		if err := i.questTracker.ValidateToken(); err != nil {
			i.logger.Warning(fmt.Sprintf("⚠️  Quest tracking validation failed: %v", err))
			i.logger.Warning("Quest tracking will be disabled for this session")
		} else {
			i.logger.Info("✅ Quest tracking initialized and validated")
		}
	}

	// Start screenshot monitoring
	if err := i.screenshotMonitor.StartMonitoring(); err != nil {
		return fmt.Errorf("failed to start screenshot monitoring: %w", err)
	}

	// Process existing files if enabled
	if i.config.AutoProcessExisting {
		if err := i.screenshotMonitor.ProcessExistingFiles(); err != nil {
			i.logger.Warning(fmt.Sprintf("Failed to process existing files: %v", err))
		}
	}

	// Always attempt to connect to WebSocket when integration is started
	// (AutoConnect only controls whether to auto-start integration at app launch)
	i.logger.Info("🌐 Connecting to tarkov.dev WebSocket...")
	if err := i.wsClient.Connect(); err != nil {
		// Connection failure is not fatal - we can retry
		i.logger.Warning(fmt.Sprintf("⚠️  Failed to connect to WebSocket: %v", err))
		i.logger.Warning("   WebSocket will attempt to reconnect automatically")
		// Emit error event but don't fail initialization
		i.emit("error", fmt.Errorf("WebSocket connection failed: %w", err))
	}

	i.isInitialized = true
	i.logger.Info("✅ Integration initialized successfully")
	i.emit("initialized", nil)

	return nil
}

// setupEventHandlers configures all event handlers
func (i *Integration) setupEventHandlers() {
	// WebSocket client events
	i.wsClient.OnEvent("connected", func(data interface{}) {
		i.logger.Info("✅ Connected to tarkov.dev remote control")
		i.emit("connected", data)
	})

	i.wsClient.OnEvent("disconnected", func(data interface{}) {
		i.logger.Warning("❌ Disconnected from tarkov.dev")
		i.emit("disconnected", data)
	})

	i.wsClient.OnEvent("error", func(data interface{}) {
		if err, ok := data.(error); ok {
			i.logger.Error(fmt.Sprintf("🔥 WebSocket error: %v", err))
		}
		i.emit("error", data)
	})

	i.wsClient.OnEvent("message", func(data interface{}) {
		i.handleRemoteMessage(data)
	})

	// Screenshot monitor events
	i.screenshotMonitor.OnEvent("ready", func(data interface{}) {
		i.logger.Debug("📁 Screenshot monitor ready")
		i.emit("monitorReady", data)
	})

	i.screenshotMonitor.OnEvent("eftScreenshot", func(data interface{}) {
		if fileInfo, ok := data.(*monitor.FileInfo); ok {
			i.handleNewScreenshot(fileInfo)
		}
	})

	i.screenshotMonitor.OnEvent("error", func(data interface{}) {
		if err, ok := data.(error); ok {
			i.logger.Error(fmt.Sprintf("📁 Screenshot monitor error: %v", err))
		}
		i.emit("error", data)
	})

	i.screenshotMonitor.OnEvent("existingFilesProcessed", func(data interface{}) {
		if count, ok := data.(int); ok {
			i.logger.Debug(fmt.Sprintf("📷 Processed %d existing screenshots", count))
		}
		i.emit("existingFilesProcessed", data)
	})

	// Game watcher events
	i.gameWatcher.OnEvent("mapLoaded", func(data interface{}) {
		if eventData, ok := data.(map[string]interface{}); ok {
			if raidInfoData, ok := eventData["raidInfo"].(game.RaidInfo); ok {
				i.logger.Info(fmt.Sprintf("🗺️  Game logs detected map: %s -> %s", raidInfoData.Map, raidInfoData.NormalizedMap))

				i.stateMutex.Lock()
				i.currentMap = raidInfoData.Map
				i.currentNormalizedMap = raidInfoData.NormalizedMap
				// Clear manual override when logs detect a map (logs are more reliable)
				if i.manualMapOverride != "" {
					i.logger.Debug("    Clearing manual map override (logs now available)")
					i.manualMapOverride = ""
				}
				i.stateMutex.Unlock()

				// Send map navigation command to tarkov.dev
				if i.wsClient.IsConnected() && raidInfoData.NormalizedMap != "" {
					i.logger.Info(fmt.Sprintf("🌐 Navigating tarkov.dev to map: %s", raidInfoData.NormalizedMap))
					if err := i.wsClient.SendMapChange(raidInfoData.NormalizedMap); err != nil {
						i.logger.Error(fmt.Sprintf("Failed to send map change: %v", err))
					}
				}

				i.emit("mapDetected", raidInfoData.NormalizedMap)
			}
		}
	})

	i.gameWatcher.OnEvent("raidStarted", func(data interface{}) {
		if eventData, ok := data.(map[string]interface{}); ok {
			if raidInfoData, ok := eventData["raidInfo"].(game.RaidInfo); ok {
				i.logger.Info(fmt.Sprintf("🎮 Raid started on %s", raidInfoData.Map))
			}
		}
		i.emit("raidStarted", data)
	})

	i.gameWatcher.OnEvent("raidEnded", func(data interface{}) {
		if eventData, ok := data.(map[string]interface{}); ok {
			if raidInfoData, ok := eventData["raidInfo"].(game.RaidInfo); ok {
				i.logger.Info(fmt.Sprintf("🏁 Raid ended on %s", raidInfoData.Map))
			}
		}

		i.stateMutex.Lock()
		i.currentMap = ""
		i.currentNormalizedMap = ""
		i.stateMutex.Unlock()

		i.emit("raidEnded", data)
	})

	i.gameWatcher.OnEvent("raidExited", func(data interface{}) {
		i.logger.Info("🚪 Raid exited via UserMatchOver")

		i.stateMutex.Lock()
		i.currentMap = ""
		i.currentNormalizedMap = ""
		i.stateMutex.Unlock()

		i.emit("raidExited", data)
	})

	i.gameWatcher.OnEvent("error", func(data interface{}) {
		if err, ok := data.(error); ok {
			i.logger.Error(fmt.Sprintf("📋 Game watcher error: %v", err))
		}
		i.emit("error", data)
	})

	// Quest tracking events
	i.gameWatcher.OnEvent("questStatusChanged", func(data interface{}) {
		if questEvent, ok := data.(*game.QuestEvent); ok {
			i.handleQuestStatusChanged(questEvent)
		}
	})
}

// handleNewScreenshot processes a new screenshot
func (i *Integration) handleNewScreenshot(fileInfo *monitor.FileInfo) {
	i.logger.Debug(fmt.Sprintf("🎯 Analyzing screenshot: %s", filepath.Base(fileInfo.Filename)))

	// Extract position data
	positionData, err := i.positionDetector.AnalyzeScreenshot(fileInfo.Filepath)
	if err != nil {
		i.logger.Warning(fmt.Sprintf("⚠️  No position data found in %s", filepath.Base(fileInfo.Filename)))
		i.stateMutex.RLock()
		currentMap := i.currentMap
		i.stateMutex.RUnlock()
		i.logger.Debug(fmt.Sprintf("    Current map from logs: %s", currentMap))

		// Delete screenshot even if no position data found
		i.deleteScreenshot(fileInfo.Filepath, "no_position_data")
		i.emit("screenshotProcessed", map[string]interface{}{
			"fileInfo": fileInfo,
			"success":  false,
			"reason":   "no_position_data",
		})
		return
	}

	i.logger.Debug(fmt.Sprintf("📍 Position detected: x=%.2f, y=%.2f, z=%.2f, rotation=%.1f, confidence=%.2f",
		positionData.X, positionData.Y, positionData.Z, positionData.Rotation, positionData.Confidence))

	// Check if we should update position (throttling)
	now := time.Now()
	throttleMs := time.Duration(i.config.PositionUpdateThrottleMs) * time.Millisecond

	i.stateMutex.RLock()
	lastUpdate := i.lastPositionUpdate
	i.stateMutex.RUnlock()

	if now.Sub(lastUpdate) < throttleMs {
		i.logger.Debug(fmt.Sprintf("⏳ Position update throttled (%dms)", i.config.PositionUpdateThrottleMs))
		i.deleteScreenshot(fileInfo.Filepath, "throttled")
		return
	}

	// Determine which map to use (priority: logs > filename > manual override)
	i.stateMutex.RLock()
	currentNormalizedMap := i.currentNormalizedMap
	currentMap := i.currentMap
	manualOverride := i.manualMapOverride
	i.stateMutex.RUnlock()

	mapToUse := currentNormalizedMap
	mapSource := "logs"

	// Fallback 1: Try filename-detected map if logs didn't provide one
	if mapToUse == "" && positionData.Map != "" {
		normalizedFromFilename := i.mapResolver.GetNormalizedName(positionData.Map)
		if normalizedFromFilename != "" {
			mapToUse = normalizedFromFilename
			mapSource = "filename"
			i.logger.Debug(fmt.Sprintf("📍 Using map from filename: %s -> %s (logs not available)", positionData.Map, normalizedFromFilename))
		}
	}

	// Fallback 2: Use manual override if still no map
	if mapToUse == "" && manualOverride != "" {
		mapToUse = manualOverride
		mapSource = "manual"
		i.logger.Debug(fmt.Sprintf("📍 Using manual map override: %s (logs and filename not available)", manualOverride))
	}

	// If still no map, drop the update
	if mapToUse == "" {
		i.logger.Warning("⚠️  No map available - set manual map override in Dashboard or enable logs")
		i.logger.Warning(fmt.Sprintf("    Current map: %s, Normalized: %s, Filename: %s, Manual: %s",
			currentMap, currentNormalizedMap, positionData.Map, manualOverride))
		i.deleteScreenshot(fileInfo.Filepath, "no_map_detected")
		i.emit("screenshotProcessed", map[string]interface{}{
			"fileInfo": fileInfo,
			"success":  false,
			"reason":   "no_map_detected",
		})
		return
	}

	// Override position data with normalized map name
	positionData.Map = mapToUse

	// Log which source was used
	if mapSource != "logs" {
		i.logger.Debug(fmt.Sprintf("    Map source: %s", mapSource))
	}

	// Send position update
	if err := i.updatePosition(positionData); err != nil {
		i.logger.Error(fmt.Sprintf("Failed to update position: %v", err))
	}

	i.stateMutex.Lock()
	i.lastPosition = positionData
	i.lastPositionUpdate = now
	i.stateMutex.Unlock()

	// Delete screenshot after successful processing
	i.deleteScreenshot(fileInfo.Filepath, "processed")

	i.emit("screenshotProcessed", map[string]interface{}{
		"fileInfo":     fileInfo,
		"positionData": positionData,
		"success":      true,
	})
}

// InjectPosition allows manual injection of position updates
func (i *Integration) InjectPosition(positionData *position.PositionData) error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	i.lastPosition = positionData
	i.lastPositionUpdate = time.Now()

	return i.updatePosition(positionData)
}

// updatePosition sends a position update to tarkov.dev and party server
func (i *Integration) updatePosition(positionData *position.PositionData) error {
	// tarkov.dev is one of three destinations, and the least available of them —
	// the connection can be down, or blocked entirely. Its failure must not stop
	// the party server and the local overlay from receiving the position, which
	// is what actually draws markers on screen. Record the error and carry on.
	var remoteErr error
	if i.wsClient == nil || !i.wsClient.IsConnected() {
		remoteErr = fmt.Errorf("WebSocket not connected")
	} else if err := i.wsClient.SendPositionUpdate(
		positionData.X,
		positionData.Y,
		positionData.Z,
		positionData.Rotation,
		positionData.Map,
	); err != nil {
		remoteErr = err
	}

	// Also send to party if connected
	if i.partyClient != nil && i.partyClient.IsInParty() {
		partyPos := &party.Position{
			X:        positionData.X,
			Y:        positionData.Y,
			Z:        positionData.Z,
			Rotation: positionData.Rotation,
		}
		if err := i.partyClient.SendPosition(positionData.Map, partyPos); err != nil {
			i.logger.Debug(fmt.Sprintf("Failed to send position to party: %v", err))
			// Don't fail the whole update if party send fails
		}
	}

	// Broadcast to local overlay server
	if i.overlayServer != nil {
		// Get normalized map name for overlay
		normalizedMap := i.mapResolver.GetNormalizedName(positionData.Map)
		if normalizedMap == "" {
			normalizedMap = positionData.Map // Fallback
		}

		i.overlayServer.Broadcast(map[string]interface{}{
			"type": "position",
			"data": map[string]interface{}{
				"x":        positionData.X,
				"y":        positionData.Y,
				"z":        positionData.Z,
				"rotation": positionData.Rotation,
				"map":      normalizedMap,
			},
		})
	}

	return remoteErr
}

// deleteScreenshot removes a processed screenshot file
func (i *Integration) deleteScreenshot(filepath, reason string) {
	if err := os.Remove(filepath); err != nil {
		i.logger.Warning(fmt.Sprintf("⚠️  Could not delete screenshot %s: %v", filepath, err))
	} else {
		i.logger.Debug(fmt.Sprintf("🗑️  Deleted screenshot: %s (%s)", filepath, reason))
	}
}

// handleRemoteMessage processes messages from tarkov.dev
func (i *Integration) handleRemoteMessage(data interface{}) {
	if message, ok := data.(map[string]interface{}); ok {
		i.logger.Debug(fmt.Sprintf("📨 Received message from tarkov.dev: %+v", message))

		if msgType, ok := message["type"].(string); ok {
			switch msgType {
			case "mapRequest":
				i.emit("mapRequested", message["map"])
			case "positionRequest":
				i.stateMutex.RLock()
				lastPos := i.lastPosition
				i.stateMutex.RUnlock()

				if lastPos != nil {
					i.updatePosition(lastPos)
				}
			}
		}
	}
}

// handleQuestStatusChanged processes quest status changes
func (i *Integration) handleQuestStatusChanged(questEvent *game.QuestEvent) {
	i.logger.Info(fmt.Sprintf("🎯 Quest status changed: %s", questEvent.Summary()))

	if i.questTracker.IsEnabled() {
		if err := i.questTracker.HandleQuestEvent(questEvent); err != nil {
			i.logger.Warning(fmt.Sprintf("⚠️  Failed to send quest update to TarkovTracker: %v", err))
		}
	}

	// Emit quest event for any external listeners
	i.emit("questStatusChanged", questEvent)
}

// Connect manually connects to tarkov.dev
func (i *Integration) Connect() error {
	if i.wsClient == nil {
		return fmt.Errorf("WebSocket client not initialized")
	}
	return i.wsClient.Connect()
}

// Disconnect manually disconnects from tarkov.dev
func (i *Integration) Disconnect() {
	if i.wsClient != nil {
		i.wsClient.Disconnect()
	}
}

// GetStatus returns the current integration status
func (i *Integration) GetStatus() map[string]interface{} {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	var lastUpdateStr string
	if !i.lastPositionUpdate.IsZero() {
		lastUpdateStr = i.lastPositionUpdate.Format(time.RFC3339)
	} else {
		lastUpdateStr = "Never"
	}

	// Get available maps
	availableMaps := []map[string]string{}
	for _, m := range i.mapResolver.GetAllMaps() {
		availableMaps = append(availableMaps, map[string]string{
			"id":          m.NormalizedName,
			"displayName": m.DisplayName,
		})
	}

	// Components are nil until Initialize() runs; report idle values instead of
	// dereferencing nil when status is requested on a non-initialized Integration.
	var connected bool
	var monitoring bool
	var screenshotStatus map[string]interface{}
	var wsStats map[string]interface{}
	var questStats map[string]interface{}
	if i.wsClient != nil {
		connected = i.wsClient.IsConnected()
		wsStats = i.wsClient.GetConnectionStats()
	}
	if i.screenshotMonitor != nil {
		monitoring = i.screenshotMonitor.IsMonitoring()
		screenshotStatus = i.screenshotMonitor.GetStatus()
	}
	if i.questTracker != nil {
		questStats = i.questTracker.GetStats()
	}

	return map[string]interface{}{
		"initialized":          i.isInitialized,
		"connected":            connected,
		"monitoring":           monitoring,
		"currentMap":           i.currentMap,
		"currentNormalizedMap": i.currentNormalizedMap,
		"manualMapOverride":    i.manualMapOverride,
		"availableMaps":        availableMaps,
		"lastPosition":         i.lastPosition,
		"lastUpdate":           lastUpdateStr,
		"screenshot":           screenshotStatus,
		"websocket":            wsStats,
		"questTracking":        questStats,
		"config": map[string]interface{}{
			"screenshotDir":           i.config.ScreenshotDir,
			"remoteIdPresent":         i.config.RemoteID != "",
			"questTrackingEnabled":    i.config.EnableQuestTracking,
			"questTrackingConfigured": i.config.IsQuestTrackingConfigured(),
		},
	}
}

// SetManualMapOverride sets a manual map selection for when logs are unavailable
func (i *Integration) SetManualMapOverride(mapName string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Normalise before storing. The dashboard selector hands back game name IDs
	// such as "streets" and "sandbox", while tarkov.dev only accepts normalized
	// names ("streets-of-tarkov", "ground-zero"). Storing the raw ID made the
	// override unusable: it is forwarded verbatim to SendMapChange and used as
	// the map on position updates, both of which reject unknown names.
	if mapName != "" {
		normalized := i.mapResolver.GetNormalizedName(mapName)
		if normalized == "" {
			i.logger.Warning(fmt.Sprintf("⚠️  Invalid map name for manual override: %s", mapName))
			return
		}
		mapName = normalized
	}

	i.manualMapOverride = mapName
	if mapName != "" {
		i.logger.Info(fmt.Sprintf("🗺️  Manual map override set to: %s", mapName))

		// Send map navigation command to tarkov.dev. wsClient is nil until
		// Initialize() runs, so guard before dereferencing it.
		if i.wsClient != nil && i.wsClient.IsConnected() {
			i.logger.Info(fmt.Sprintf("🌐 Navigating tarkov.dev to map: %s", mapName))
			if err := i.wsClient.SendMapChange(mapName); err != nil {
				i.logger.Error(fmt.Sprintf("Failed to send map change: %v", err))
			}
		}
	} else {
		i.logger.Debug("🗺️  Manual map override cleared")
	}
}

// GetManualMapOverride returns the current manual map override
func (i *Integration) GetManualMapOverride() string {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.manualMapOverride
}

// GetOverlayServer returns the overlay server instance for broadcasting to userscript
func (i *Integration) GetOverlayServer() *overlay.Server {
	return i.overlayServer
}

// GetMapResolver returns the map resolver for normalizing map names
func (i *Integration) GetMapResolver() *game.MapResolver {
	return i.mapResolver
}

// ConnectToParty connects to a party server and joins a party
func (i *Integration) ConnectToParty(serverURL, partyCode, displayName string) error {
	// Disconnect any existing party connection first
	if i.partyClient != nil {
		i.logger.Info("Disconnecting from previous party before joining new one...")
		i.partyClient.Disconnect()
		i.partyClient = nil
	}

	// Create fresh party client
	i.logger.Info(fmt.Sprintf("Creating party client for server: %s", serverURL))
	i.partyClient = party.NewClient(serverURL, i.logger)

	// IMPORTANT: Setup event handlers BEFORE connecting
	// This ensures handlers are ready when messages arrive
	i.setupPartyEventHandlers()

	// Connect to the party server
	i.logger.Info("Connecting to party server...")
	if err := i.partyClient.Connect(); err != nil {
		i.partyClient = nil
		return fmt.Errorf("failed to connect to party server: %w", err)
	}

	// Join the party (this now waits for server response)
	i.logger.Info(fmt.Sprintf("Sending join request for party: %s", partyCode))
	if err := i.partyClient.JoinParty(partyCode, i.config.RemoteID, displayName); err != nil {
		// Join failed - clean up connection
		i.partyClient.Disconnect()
		i.partyClient = nil
		return fmt.Errorf("failed to join party: %w", err)
	}

	i.logger.Info(fmt.Sprintf("✅ Successfully joined party: %s", partyCode))
	return nil
}

// DisconnectFromParty disconnects from the party server
func (i *Integration) DisconnectFromParty() error {
	if i.partyClient == nil {
		return nil
	}

	if err := i.partyClient.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect from party: %w", err)
	}

	i.logger.Info("👋 Disconnected from party")
	return nil
}

// IsInParty returns whether the integration is currently in a party
func (i *Integration) IsInParty() bool {
	return i.partyClient != nil && i.partyClient.IsInParty()
}

// GetPartyCode returns the current party code
func (i *Integration) GetPartyCode() string {
	if i.partyClient == nil {
		return ""
	}
	return i.partyClient.GetPartyCode()
}

// setupPartyEventHandlers sets up event handlers for party events
func (i *Integration) setupPartyEventHandlers() {
	if i.partyClient == nil {
		return
	}

	// Create party event handler
	handler := &partyEventHandler{
		integration: i,
	}
	i.partyClient.SetEventHandler(handler)
}

// partyEventHandler implements party.EventHandler
type partyEventHandler struct {
	integration *Integration
}

func (h *partyEventHandler) OnWelcome(msg *party.WelcomeMessage) {
	h.integration.logger.Info(fmt.Sprintf("🎉 Joined party: %s with %d members", msg.PartyCode, len(msg.Members)))
	h.integration.emit("party:welcome", msg)
}

func (h *partyEventHandler) OnMemberJoined(msg *party.MemberJoinedMessage) {
	h.integration.logger.Info(fmt.Sprintf("👥 %s joined the party", msg.Member.DisplayName))
	h.integration.emit("party:memberJoined", msg)
}

func (h *partyEventHandler) OnMemberLeft(msg *party.MemberLeftMessage) {
	h.integration.logger.Info(fmt.Sprintf("👋 Member %s left the party", msg.RemoteID))

	// Clear the marker on the local overlay. This used to be forwarded to
	// tarkov.dev instead, which both used a message type their server does not
	// define and left the overlay marker stuck on screen.
	if h.integration.overlayServer != nil {
		h.integration.overlayServer.Broadcast(map[string]interface{}{
			"type": "partyMemberLeft",
			"data": map[string]interface{}{
				"remoteId": msg.RemoteID,
			},
		})
	}

	h.integration.emit("party:memberLeft", msg)
}

func (h *partyEventHandler) OnPositionUpdate(msg *party.PositionUpdateMessage) {
	h.integration.logger.Info(fmt.Sprintf("📍 Received position update from %s on %s at (%.2f, %.2f, %.2f)", msg.DisplayName, msg.Map, msg.Position.X, msg.Position.Y, msg.Position.Z))
	// Party positions go to the local overlay only — never to tarkov.dev.
	// Broadcast to local overlay server
	if h.integration.overlayServer != nil {
		// Get normalized map name for overlay
		normalizedMap := h.integration.mapResolver.GetNormalizedName(msg.Map)
		if normalizedMap == "" {
			normalizedMap = msg.Map // Fallback
		}

		h.integration.logger.Debug(fmt.Sprintf("Broadcasting position to overlay: %s on %s (norm: %s)", msg.DisplayName, msg.Map, normalizedMap))

		h.integration.overlayServer.Broadcast(map[string]interface{}{
			"type": "partyPosition",
			"data": map[string]interface{}{
				"remoteId":    msg.RemoteID,
				"displayName": msg.DisplayName,
				"position": map[string]interface{}{
					"x": msg.Position.X,
					"y": msg.Position.Y,
					"z": msg.Position.Z,
				},
				"rotation": msg.Position.Rotation,
				"map":      normalizedMap,
			},
		})
	}

	h.integration.emit("party:positionReceived", msg)
}

func (h *partyEventHandler) OnError(msg *party.ErrorMessage) {
	h.integration.logger.Error(fmt.Sprintf("❌ Party error [%s]: %s", msg.Code, msg.Message))
	h.integration.emit("party:error", msg)
}

func (h *partyEventHandler) OnDisconnected() {
	h.integration.logger.Warning("⚠️  Disconnected from party server")
	h.integration.emit("party:disconnected", nil)
}

// Shutdown gracefully shuts down all components
func (i *Integration) Shutdown() error {
	i.logger.Info("🛑 Shutting down integration...")

	i.screenshotMonitor.StopMonitoring()
	i.wsClient.Disconnect()
	i.gameWatcher.Stop()
	i.questTracker.Close()

	// Disconnect from party if connected
	if i.partyClient != nil {
		i.partyClient.Disconnect()
	}

	i.cancel()
	i.isInitialized = false

	i.logger.Info("✅ Shutdown complete")
	i.emit("shutdown", nil)

	// Clear event handlers to prevent memory leaks on restart
	i.ClearEventHandlers()

	return nil
}

// OnEvent registers an event handler
func (i *Integration) OnEvent(event string, handler func(interface{})) {
	i.handlerMutex.Lock()
	defer i.handlerMutex.Unlock()

	i.eventHandlers[event] = append(i.eventHandlers[event], handler)
}

// ClearEventHandlers removes all registered event handlers to prevent memory leaks
func (i *Integration) ClearEventHandlers() {
	i.handlerMutex.Lock()
	defer i.handlerMutex.Unlock()

	i.eventHandlers = make(map[string][]func(interface{}))
}

// emit triggers event handlers
func (i *Integration) emit(event string, data interface{}) {
	i.handlerMutex.RLock()
	defer i.handlerMutex.RUnlock()

	if handlers, exists := i.eventHandlers[event]; exists {
		for _, handler := range handlers {
			go func(h func(interface{}), ev string) {
				defer func() {
					if r := recover(); r != nil {
						i.logger.Error(fmt.Sprintf("Panic in integration event handler [%s]: %v", ev, r))
					}
				}()
				h(data)
			}(handler, event)
		}
	}
}
