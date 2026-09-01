package game

import (
	"testing"
)

// TestQuestDetectionFromActualLog tests quest detection from the actual quest completion log
func TestQuestDetectionFromActualLog(t *testing.T) {
	// Real quest completion log from 2025.07.18_14-32-10_0.16.8.1.38114 notifications.log
	actualQuestLog := `2025-07-19 00:39:06.581 +10:00|Info|push-notifications|Got notification | ChatMessageReceived
{
  "type": "new_message",
  "eventId": "687a5c89e8573436de06f006",
  "dialogId": "5a7c2eca46aef81a7ca2145d",
  "message": {
    "_id": "687a5c8967d89c5b720aa47527",
    "uid": "5a7c2eca46aef81a7ca2145d",
    "type": 12,
    "dt": 1752849545,
    "text": "",
    "templateId": "5ac23c6186f7741247042bad successMessageText",
    "items": {
      "stash": "687a5c8967d89c5b720aa476",
      "data": [
        {
          "_id": "687a5c8967d89c5b720aa46a",
          "_tpl": "5449016a4bdc2d6f028b456f",
          "upd": {
            "StackObjectsCount": 10000
          },
          "parentId": "687a5c8967d89c5b720aa476",
          "slotId": "main"
        }
      ]
    },
    "profileChangeEvents": [],
    "hasRewards": true
  }
}`

	// Test quest detection
	t.Run("DetectActualQuestCompletion", func(t *testing.T) {
		if !IsQuestNotification(actualQuestLog) {
			t.Error("Should detect actual quest completion notification")
		}
	})

	// Test quest parsing
	t.Run("ParseActualQuestCompletion", func(t *testing.T) {
		questEvent, err := ParseQuestNotification(actualQuestLog)
		if err != nil {
			t.Fatalf("Failed to parse actual quest completion: %v", err)
		}

		expectedQuestID := "5ac23c6186f7741247042bad"
		if questEvent.QuestID != expectedQuestID {
			t.Errorf("Expected quest ID %s, got %s", expectedQuestID, questEvent.QuestID)
		}

		if questEvent.Status != QuestFinished {
			t.Errorf("Expected QuestFinished status, got %v", questEvent.Status)
		}

		if questEvent.Status.String() != "completed" {
			t.Errorf("Expected 'completed' status string, got %s", questEvent.Status.String())
		}

		t.Logf("Successfully parsed quest: %s", questEvent.Summary())
	})
}