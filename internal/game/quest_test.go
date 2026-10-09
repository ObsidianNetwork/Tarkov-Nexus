package game

import (
	"testing"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
)

type discardLogger struct{}

func (discardLogger) Debug(string)   {}
func (discardLogger) Info(string)    {}
func (discardLogger) Warning(string) {}
func (discardLogger) Error(string)   {}

func TestRaidStateFailsClosedOnInitialReadAndTracksLiveTransitions(t *testing.T) {
	var log logger.Logger = discardLogger{}
	watcher := NewWatcher(log)
	startedAt := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// Old GameStarted lines are deliberately ignored during initial log replay.
	watcher.processLogEvent("application|GameStarted", startedAt, true)
	if got := watcher.GetRaidInfo(); got.StartedTime != nil {
		t.Fatal("initial log replay incorrectly restored an old active raid")
	}

	watcher.processLogEvent("application|GameStarted", startedAt, false)
	if got := watcher.GetRaidInfo(); got.StartedTime == nil || got.EndedTime != nil {
		t.Fatalf("live GameStarted did not mark raid active: %+v", got)
	}

	endedAt := startedAt.Add(time.Minute)
	watcher.processLogEvent("Got notification | UserMatchOver", endedAt, false)
	if got := watcher.GetRaidInfo(); got.StartedTime != nil || got.EndedTime != nil {
		t.Fatalf("raid exit did not clear active raid state: %+v", got)
	}

	watcher.processLogEvent("application|GameStarted", endedAt, false)
	watcher.processLogEvent("SelectProfile ProfileId:player123", endedAt.Add(time.Minute), false)
	if got := watcher.GetRaidInfo(); got.StartedTime == nil || got.EndedTime == nil {
		t.Fatalf("live profile selection did not mark raid ended: %+v", got)
	}

	// These lifecycle signals are observed in logs even when the usual match-end
	// notification is missing, so they must also fail closed for automation.
	watcher.processLogEvent("application|GameStarted", endedAt.Add(2*time.Minute), false)
	watcher.processLogEvent("application|Session.OnDisconnect", endedAt.Add(3*time.Minute), false)
	if got := watcher.GetRaidInfo(); got.StartedTime != nil {
		t.Fatalf("Session.OnDisconnect left raid state active: %+v", got)
	}

	watcher.processLogEvent("application|GameStarted", endedAt.Add(4*time.Minute), false)
	watcher.processLogEvent("application|PrepareSelectedProfileLocally", endedAt.Add(5*time.Minute), false)
	if got := watcher.GetRaidInfo(); got.StartedTime != nil {
		t.Fatalf("PrepareSelectedProfileLocally left raid state active: %+v", got)
	}
}

func TestIsQuestNotification(t *testing.T) {
	testCases := []struct {
		name     string
		logLine  string
		expected bool
	}{
		{
			name:     "Valid quest started notification",
			logLine:  `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 10, "templateId": "quest123 additional_data", "text": "Quest started"}}`,
			expected: true,
		},
		{
			name:     "Valid quest failed notification",
			logLine:  `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 11, "templateId": "quest456", "text": "Quest failed"}}`,
			expected: true,
		},
		{
			name:     "Valid quest completed notification",
			logLine:  `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 12, "templateId": "quest789", "text": "Quest completed"}}`,
			expected: true,
		},
		{
			name:     "Non-quest chat message",
			logLine:  `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 5, "templateId": "msg123", "text": "Regular message"}}`,
			expected: false,
		},
		{
			name:     "Non-chat notification",
			logLine:  `2024-01-15 14:30:25.123 +03:00|Got notification | SomeOtherNotification {"data": "test"}`,
			expected: false,
		},
		{
			name:     "Regular log line",
			logLine:  `2024-01-15 14:30:25.123 +03:00|application|Some regular log message`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsQuestNotification(tc.logLine)
			if result != tc.expected {
				t.Errorf("IsQuestNotification(%q) = %v, expected %v", tc.logLine, result, tc.expected)
			}
		})
	}
}

func TestParseQuestNotification(t *testing.T) {
	testCases := []struct {
		name           string
		logLine        string
		expectedQuestID string
		expectedStatus  QuestStatus
		expectError    bool
	}{
		{
			name:           "Quest started",
			logLine:        `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 10, "templateId": "quest123 additional_data", "text": "Quest started"}}`,
			expectedQuestID: "quest123",
			expectedStatus:  QuestStarted,
			expectError:    false,
		},
		{
			name:           "Quest failed",
			logLine:        `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 11, "templateId": "quest456", "text": "Quest failed"}}`,
			expectedQuestID: "quest456",
			expectedStatus:  QuestFailed,
			expectError:    false,
		},
		{
			name:           "Quest completed",
			logLine:        `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {"message": {"type": 12, "templateId": "quest789", "text": "Quest completed"}}`,
			expectedQuestID: "quest789",
			expectedStatus:  QuestFinished,
			expectError:    false,
		},
		{
			name:        "Invalid JSON",
			logLine:     `2024-01-15 14:30:25.123 +03:00|Got notification | ChatMessageReceived {invalid json}`,
			expectError: true,
		},
		{
			name:        "No quest notification",
			logLine:     `2024-01-15 14:30:25.123 +03:00|Some other log line`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			questEvent, err := ParseQuestNotification(tc.logLine)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("ParseQuestNotification(%q) expected error but got none", tc.logLine)
				}
				return
			}
			
			if err != nil {
				t.Errorf("ParseQuestNotification(%q) unexpected error: %v", tc.logLine, err)
				return
			}
			
			if questEvent.QuestID != tc.expectedQuestID {
				t.Errorf("ParseQuestNotification(%q) QuestID = %q, expected %q", tc.logLine, questEvent.QuestID, tc.expectedQuestID)
			}
			
			if questEvent.Status != tc.expectedStatus {
				t.Errorf("ParseQuestNotification(%q) Status = %v, expected %v", tc.logLine, questEvent.Status, tc.expectedStatus)
			}
		})
	}
}

func TestQuestStatusString(t *testing.T) {
	testCases := []struct {
		status   QuestStatus
		expected string
	}{
		{QuestStarted, "uncompleted"},
		{QuestFailed, "failed"},
		{QuestFinished, "completed"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.status.String()
			if result != tc.expected {
				t.Errorf("QuestStatus(%v).String() = %q, expected %q", tc.status, result, tc.expected)
			}
		})
	}
}

func TestQuestStatusFromInt(t *testing.T) {
	testCases := []struct {
		input       int
		expected    QuestStatus
		expectError bool
	}{
		{10, QuestStarted, false},
		{11, QuestFailed, false},
		{12, QuestFinished, false},
		{5, QuestStarted, true},  // Invalid type should return error
		{20, QuestStarted, true}, // Invalid type should return error
	}

	for _, tc := range testCases {
		t.Run(tc.expected.String(), func(t *testing.T) {
			result, err := QuestStatusFromInt(tc.input)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("QuestStatusFromInt(%d) expected error but got none", tc.input)
				}
				return
			}
			
			if err != nil {
				t.Errorf("QuestStatusFromInt(%d) unexpected error: %v", tc.input, err)
				return
			}
			
			if result != tc.expected {
				t.Errorf("QuestStatusFromInt(%d) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}
