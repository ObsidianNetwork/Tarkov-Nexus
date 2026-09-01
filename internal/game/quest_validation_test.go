package game

import (
	"testing"
	"time"
)

// TestQuestParsingWithRealData tests quest parsing with real EFT log data
func TestQuestParsingWithRealData(t *testing.T) {
	// Real quest started message from the log
	questStartedLog := `2025-07-10 22:01:46.289 +10:00|Info|push-notifications|Got notification | ChatMessageReceived
{
  "type": "new_message",
  "eventId": "686faba8b856f2508602f8b2",
  "dialogId": "58330581ace78e27b8b10cee",
  "message": {
    "_id": "686faba884766ad7c406209d",
    "uid": "58330581ace78e27b8b10cee",
    "type": 10,
    "dt": 1752148904,
    "text": "",
    "templateId": "596b43fb86f77457ca186186 description",
    "hasRewards": false,
    "maxStorageTime": 604800
  }
}`

	// Real quest completed message from the log  
	questCompletedLog := `2025-07-10 22:04:19.019 +10:00|Info|push-notifications|Got notification | ChatMessageReceived
{
  "type": "new_message",
  "eventId": "686fb3eba5892d997408609c",
  "dialogId": "58330581ace78e27b8b10cee",
  "message": {
    "_id": "686fb3eba33ff107970cc5c028",
    "uid": "58330581ace78e27b8b10cee",
    "type": 12,
    "dt": 1752151019,
    "text": "",
    "templateId": "596b43fb86f77457ca186186 successMessageText",
    "items": {
      "stash": "686fb3eba33ff107970cc5c1"
    },
    "hasRewards": true
  }
}`

	// Test quest started detection
	t.Run("DetectQuestStarted", func(t *testing.T) {
		if !IsQuestNotification(questStartedLog) {
			t.Error("Should detect quest started notification")
		}
	})

	// Test quest completed detection
	t.Run("DetectQuestCompleted", func(t *testing.T) {
		if !IsQuestNotification(questCompletedLog) {
			t.Error("Should detect quest completed notification")
		}
	})

	// Test quest started parsing
	t.Run("ParseQuestStarted", func(t *testing.T) {
		questEvent, err := ParseQuestNotification(questStartedLog)
		if err != nil {
			t.Fatalf("Failed to parse quest started: %v", err)
		}

		expectedQuestID := "596b43fb86f77457ca186186"
		if questEvent.QuestID != expectedQuestID {
			t.Errorf("Expected quest ID %s, got %s", expectedQuestID, questEvent.QuestID)
		}

		if questEvent.Status != QuestStarted {
			t.Errorf("Expected QuestStarted status, got %v", questEvent.Status)
		}

		if questEvent.Status.String() != "uncompleted" {
			t.Errorf("Expected 'uncompleted' status string, got %s", questEvent.Status.String())
		}
	})

	// Test quest completed parsing
	t.Run("ParseQuestCompleted", func(t *testing.T) {
		questEvent, err := ParseQuestNotification(questCompletedLog)
		if err != nil {
			t.Fatalf("Failed to parse quest completed: %v", err)
		}

		expectedQuestID := "596b43fb86f77457ca186186"
		if questEvent.QuestID != expectedQuestID {
			t.Errorf("Expected quest ID %s, got %s", expectedQuestID, questEvent.QuestID)
		}

		if questEvent.Status != QuestFinished {
			t.Errorf("Expected QuestFinished status, got %v", questEvent.Status)
		}

		if questEvent.Status.String() != "completed" {
			t.Errorf("Expected 'completed' status string, got %s", questEvent.Status.String())
		}
	})

	// Test quest event summary
	t.Run("QuestEventSummary", func(t *testing.T) {
		questEvent := &QuestEvent{
			QuestID:   "596b43fb86f77457ca186186",
			Status:    QuestStarted,
			Timestamp: time.Now(),
		}

		summary := questEvent.Summary()
		expectedSummary := "Quest 596b43fb86f77457ca186186: Quest Started (uncompleted)"
		if summary != expectedSummary {
			t.Errorf("Expected summary '%s', got '%s'", expectedSummary, summary)
		}
	})
}

// TestQuestIDExtraction tests various quest ID extraction scenarios
func TestQuestIDExtraction(t *testing.T) {
	tests := []struct {
		name       string
		templateID string
		expected   string
	}{
		{
			name:       "Quest with description",
			templateID: "596b43fb86f77457ca186186 description",
			expected:   "596b43fb86f77457ca186186",
		},
		{
			name:       "Quest with successMessageText",
			templateID: "596b43fb86f77457ca186186 successMessageText",
			expected:   "596b43fb86f77457ca186186",
		},
		{
			name:       "Quest with complex template",
			templateID: "616051e63f96cc089c1cf37f successMessageText 58330581ace78e27b8b10cee 0",
			expected:   "616051e63f96cc089c1cf37f",
		},
		{
			name:       "Quest ID only",
			templateID: "59689fbd86f7740d137ebfc4",
			expected:   "59689fbd86f7740d137ebfc4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notification := &QuestNotification{
				Message: ChatMessage{
					TemplateID: tt.templateID,
				},
			}

			result := notification.ExtractQuestID()
			if result != tt.expected {
				t.Errorf("Expected quest ID '%s', got '%s'", tt.expected, result)
			}
		})
	}
}