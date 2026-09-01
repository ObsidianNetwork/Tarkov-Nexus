package tracker

import (
	"fmt"
	"tarkov-screenshot-analyzer/internal/game"
	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/storage"
)

// QuestTracker manages quest tracking with TarkovTracker API
type QuestTracker struct {
	client  *Client
	enabled bool
	storage *storage.QuestStorage
	logger  logger.Logger
}

// NewQuestTracker creates a new quest tracker
func NewQuestTracker(config Config, enabled bool, log logger.Logger) *QuestTracker {
	if !enabled {
		return &QuestTracker{enabled: false, logger: log}
	}

	client := NewClient(config)

	// Initialize quest storage
	questStorage, err := storage.NewQuestStorage()
	if err != nil {
		log.Warning(fmt.Sprintf("Warning: Failed to initialize quest storage: %v", err))
		// Continue without storage rather than failing completely
	}

	return &QuestTracker{
		client:  client,
		enabled: true,
		storage: questStorage,
		logger:  log,
	}
}

// IsEnabled returns true if quest tracking is enabled
func (qt *QuestTracker) IsEnabled() bool {
	return qt.enabled
}

// ValidateToken validates the API token and permissions
func (qt *QuestTracker) ValidateToken() error {
	if !qt.enabled {
		return fmt.Errorf("quest tracking is disabled")
	}

	valid, err := qt.client.IsValidToken()
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if !valid {
		return fmt.Errorf("token is invalid or lacks required permissions")
	}

	return nil
}

// HandleQuestEvent processes a quest event from the game watcher
func (qt *QuestTracker) HandleQuestEvent(questEvent *game.QuestEvent) error {
	if !qt.enabled {
		return nil // Silently skip if disabled
	}

	if qt.client == nil {
		return fmt.Errorf("TarkovTracker client not initialized")
	}

	// Only process completed quests for live tracking
	if questEvent.Status.String() != "completed" {
		qt.logger.Debug(fmt.Sprintf("🔄 Skipping non-completion in live tracking: %s -> %s", questEvent.QuestID, questEvent.Status.String()))
		return nil
	}

	// Check if already processed (avoid duplicates)
	if qt.storage != nil && qt.storage.HasProcessed(questEvent.QuestID, questEvent.Status.String()) {
		qt.logger.Debug(fmt.Sprintf("💾 Skipping already processed quest in live tracking: %s -> %s", questEvent.QuestID, questEvent.Status.String()))
		return nil
	}

	// Convert quest status to TarkovTracker API format
	apiStatus := questEvent.Status.String()

	qt.logger.Debug(fmt.Sprintf("📤 Sending quest update to TarkovTracker: %s -> %s", questEvent.QuestID, apiStatus))

	// Send update to TarkovTracker API (with rate limiting for live tracking)
	if err := qt.client.UpdateTaskStatus(questEvent.QuestID, apiStatus); err != nil {
		return fmt.Errorf("failed to update quest status: %w", err)
	}

	// Mark as processed in storage
	if qt.storage != nil {
		if err := qt.storage.MarkProcessed(questEvent.QuestID, questEvent.Status.String()); err != nil {
			qt.logger.Warning(fmt.Sprintf("⚠️ Failed to save to storage: %v", err))
		}
	}

	qt.logger.Debug(fmt.Sprintf("✅ Successfully updated quest %s in TarkovTracker", questEvent.QuestID))
	return nil
}

// HandleQuestEventFast processes a quest event without rate limiting (for bulk operations)
func (qt *QuestTracker) HandleQuestEventFast(questEvent *game.QuestEvent) error {
	if !qt.enabled {
		return nil // Silently skip if disabled
	}

	if qt.client == nil {
		return fmt.Errorf("TarkovTracker client not initialized")
	}

	// Only process completed quests for live tracking
	if questEvent.Status.String() != "completed" {
		qt.logger.Debug(fmt.Sprintf("🔄 Skipping non-completion in bulk processing: %s -> %s", questEvent.QuestID, questEvent.Status.String()))
		return nil
	}

	// Check if already processed (avoid duplicates)
	if qt.storage != nil && qt.storage.HasProcessed(questEvent.QuestID, questEvent.Status.String()) {
		qt.logger.Debug(fmt.Sprintf("💾 Skipping already processed quest in bulk processing: %s -> %s", questEvent.QuestID, questEvent.Status.String()))
		return nil
	}

	// Convert quest status to TarkovTracker API format
	apiStatus := questEvent.Status.String()

	qt.logger.Debug(fmt.Sprintf("📤 Sending quest update to TarkovTracker (no rate limit): %s -> %s", questEvent.QuestID, apiStatus))

	// Send update to TarkovTracker API without rate limiting
	if err := qt.client.UpdateTaskStatusWithRateLimit(questEvent.QuestID, apiStatus, false); err != nil {
		return fmt.Errorf("failed to update quest status: %w", err)
	}

	// Mark as processed in storage
	if qt.storage != nil {
		if err := qt.storage.MarkProcessed(questEvent.QuestID, questEvent.Status.String()); err != nil {
			qt.logger.Warning(fmt.Sprintf("⚠️ Failed to save to storage: %v", err))
		}
	}

	qt.logger.Debug(fmt.Sprintf("✅ Successfully updated quest %s in TarkovTracker (fast)", questEvent.QuestID))
	return nil
}

// GetProgress retrieves current progress from TarkovTracker
func (qt *QuestTracker) GetProgress(gameMode string) (*ProgressResponse, error) {
	if !qt.enabled {
		return nil, fmt.Errorf("quest tracking is disabled")
	}

	if qt.client == nil {
		return nil, fmt.Errorf("TarkovTracker client not initialized")
	}

	return qt.client.GetProgress(gameMode)
}

// GetStats returns quest tracker statistics
func (qt *QuestTracker) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled": qt.enabled,
	}

	if qt.client != nil {
		clientStats := qt.client.GetConnectionStats()
		for k, v := range clientStats {
			stats[k] = v
		}
	}

	return stats
}

// Close shuts down the quest tracker
func (qt *QuestTracker) Close() {
	if qt.client != nil {
		qt.client.Close()
	}
}

// BatchUpdateQuests updates multiple quest statuses at once
func (qt *QuestTracker) BatchUpdateQuests(questEvents []*game.QuestEvent) error {
	if !qt.enabled {
		return nil
	}

	if qt.client == nil {
		return fmt.Errorf("TarkovTracker client not initialized")
	}

	if len(questEvents) == 0 {
		return nil
	}

	// Convert quest events to batch update format
	updates := make([]TaskStatusBatchUpdate, len(questEvents))
	for i, event := range questEvents {
		updates[i] = TaskStatusBatchUpdate{
			ID:    event.QuestID,
			State: event.Status.String(),
		}
	}

	qt.logger.Debug(fmt.Sprintf("📤 Sending batch quest update to TarkovTracker: %d quests", len(updates)))

	if err := qt.client.UpdateTaskStatuses(updates); err != nil {
		return fmt.Errorf("failed to batch update quest statuses: %w", err)
	}

	qt.logger.Debug(fmt.Sprintf("✅ Successfully updated %d quests in TarkovTracker", len(updates)))
	return nil
}