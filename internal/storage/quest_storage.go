package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ProcessedQuest represents a quest that has been processed
type ProcessedQuest struct {
	QuestID     string    `json:"questId"`
	Status      string    `json:"status"`
	ProcessedAt time.Time `json:"processedAt"`
}

// QuestStorage handles persistent storage of processed quests
type QuestStorage struct {
	filePath string
	quests   map[string]ProcessedQuest
	mutex    sync.RWMutex
}

// NewQuestStorage creates a new quest storage instance
func NewQuestStorage() (*QuestStorage, error) {
	// Get program directory
	exeDir, err := os.Executable()
	if err != nil {
		// Fallback to current working directory
		exeDir, _ = os.Getwd()
	}
	programDir := filepath.Dir(exeDir)
	
	filePath := filepath.Join(programDir, "processed_quests.json")
	
	qs := &QuestStorage{
		filePath: filePath,
		quests:   make(map[string]ProcessedQuest),
	}
	
	// Load existing data
	if err := qs.load(); err != nil {
		// If file doesn't exist or can't be loaded, start fresh
		fmt.Printf("Creating new quest storage at: %s\n", filePath)
	}
	
	return qs, nil
}

// HasProcessed checks if a quest completion has already been processed
func (qs *QuestStorage) HasProcessed(questID, status string) bool {
	qs.mutex.RLock()
	defer qs.mutex.RUnlock()
	
	key := fmt.Sprintf("%s_%s", questID, status)
	_, exists := qs.quests[key]
	return exists
}

// MarkProcessed marks a quest completion as processed
func (qs *QuestStorage) MarkProcessed(questID, status string) error {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()
	
	key := fmt.Sprintf("%s_%s", questID, status)
	qs.quests[key] = ProcessedQuest{
		QuestID:     questID,
		Status:      status,
		ProcessedAt: time.Now(),
	}
	
	return qs.save()
}

// GetProcessedCount returns the number of processed quest completions
func (qs *QuestStorage) GetProcessedCount() int {
	qs.mutex.RLock()
	defer qs.mutex.RUnlock()
	return len(qs.quests)
}

// GetProcessedQuests returns all processed quests
func (qs *QuestStorage) GetProcessedQuests() []ProcessedQuest {
	qs.mutex.RLock()
	defer qs.mutex.RUnlock()
	
	quests := make([]ProcessedQuest, 0, len(qs.quests))
	for _, quest := range qs.quests {
		quests = append(quests, quest)
	}
	return quests
}

// ClearOldEntries removes processed quests older than the specified duration
func (qs *QuestStorage) ClearOldEntries(maxAge time.Duration) error {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()
	
	cutoff := time.Now().Add(-maxAge)
	updated := false
	
	for key, quest := range qs.quests {
		if quest.ProcessedAt.Before(cutoff) {
			delete(qs.quests, key)
			updated = true
		}
	}
	
	if updated {
		return qs.save()
	}
	return nil
}

// load reads the quest storage from disk
func (qs *QuestStorage) load() error {
	data, err := os.ReadFile(qs.filePath)
	if err != nil {
		return err
	}
	
	var quests map[string]ProcessedQuest
	if err := json.Unmarshal(data, &quests); err != nil {
		return err
	}
	
	qs.quests = quests
	fmt.Printf("Loaded %d processed quest records from storage\n", len(qs.quests))
	return nil
}

// save writes the quest storage to disk
func (qs *QuestStorage) save() error {
	data, err := json.MarshalIndent(qs.quests, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(qs.filePath, data, 0644)
}

// Reset clears all processed quest records
func (qs *QuestStorage) Reset() error {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()
	
	qs.quests = make(map[string]ProcessedQuest)
	return qs.save()
}