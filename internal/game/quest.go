package game

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// QuestEvent represents a quest status change detected from EFT logs
type QuestEvent struct {
	QuestID   string      `json:"questId"`
	Status    QuestStatus `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
	RawData   interface{} `json:"rawData,omitempty"`
}

// QuestStatus represents quest completion status
type QuestStatus int

const (
	QuestStarted QuestStatus = iota + 10 // Match EFT log message types
	QuestFailed
	QuestFinished
)

// String returns the TarkovTracker API-compatible string representation
func (qs QuestStatus) String() string {
	switch qs {
	case QuestStarted:
		return "uncompleted"
	case QuestFailed:
		return "failed"
	case QuestFinished:
		return "completed"
	default:
		return "uncompleted"
	}
}

// FromInt converts EFT log message type to QuestStatus
func QuestStatusFromInt(messageType int) (QuestStatus, error) {
	switch messageType {
	case 10:
		return QuestStarted, nil
	case 11:
		return QuestFailed, nil
	case 12:
		return QuestFinished, nil
	default:
		return QuestStarted, fmt.Errorf("unknown quest message type: %d", messageType)
	}
}

// ChatMessage represents the structure of chat messages in EFT logs
type ChatMessage struct {
	Type       int         `json:"type"`
	TemplateID string      `json:"templateId"`
	Text       string      `json:"text"`
	HasRewards bool        `json:"hasRewards,omitempty"`
	Items      interface{} `json:"items,omitempty"`
}

// QuestNotification represents the full quest notification structure
type QuestNotification struct {
	Message ChatMessage `json:"message"`
}

// ExtractQuestID extracts quest ID from templateId field
func (qn *QuestNotification) ExtractQuestID() string {
	// Quest ID is typically the first part before space in templateId
	parts := strings.Split(qn.Message.TemplateID, " ")
	if len(parts) > 0 {
		return parts[0]
	}
	return qn.Message.TemplateID
}

// ParseQuestNotification parses quest notification from EFT log text (can be multiline)
func ParseQuestNotification(logText string) (*QuestEvent, error) {
	// Find the start of the JSON object
	startIndex := strings.Index(logText, "{")
	if startIndex == -1 {
		return nil, fmt.Errorf("no JSON object found in log text")
	}
	
	// Extract the JSON part (everything from the first { to the end)
	jsonPart := logText[startIndex:]
	
	// Look for quest notification pattern within the JSON
	if !strings.Contains(jsonPart, `"message":`) {
		return nil, fmt.Errorf("no message field found in JSON")
	}
	
	// Check if it contains quest type (10, 11, or 12) somewhere in the JSON
	// We'll validate the exact structure after parsing
	questTypeRegex := regexp.MustCompile(`"type":\s*(?:10|11|12)`)
	if !questTypeRegex.MatchString(jsonPart) {
		return nil, fmt.Errorf("no quest type found in JSON")
	}
	
	// Try to find the complete JSON object by matching braces
	jsonMatch := extractCompleteJSON(jsonPart)
	if jsonMatch == "" {
		return nil, fmt.Errorf("could not extract complete JSON object")
	}

	var notification QuestNotification
	if err := json.Unmarshal([]byte(jsonMatch), &notification); err != nil {
		return nil, fmt.Errorf("failed to parse quest notification JSON: %w", err)
	}

	// Validate message type is quest-related
	if notification.Message.Type < 10 || notification.Message.Type > 12 {
		return nil, fmt.Errorf("message type %d is not a quest notification", notification.Message.Type)
	}

	status, err := QuestStatusFromInt(notification.Message.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message type to quest status: %w", err)
	}

	questID := notification.ExtractQuestID()
	if questID == "" {
		return nil, fmt.Errorf("could not extract quest ID from templateId: %s", notification.Message.TemplateID)
	}

	return &QuestEvent{
		QuestID:   questID,
		Status:    status,
		Timestamp: time.Now(),
		RawData:   notification,
	}, nil
}

// IsQuestNotification checks if a log line contains a quest notification
func IsQuestNotification(logLine string) bool {
	// Check for quest-related chat notification patterns
	if !strings.Contains(logLine, "Got notification | ChatMessageReceived") {
		return false
	}

	// Check for quest message types (10, 11, 12)
	questTypeRegex := regexp.MustCompile(`"type":\s*(?:10|11|12)`)
	return questTypeRegex.MatchString(logLine)
}

// GetQuestStatusDescription returns human-readable quest status description
func (qs QuestStatus) GetDescription() string {
	switch qs {
	case QuestStarted:
		return "Quest Started"
	case QuestFailed:
		return "Quest Failed"
	case QuestFinished:
		return "Quest Completed"
	default:
		return "Unknown Status"
	}
}

// QuestEventSummary provides a summary of the quest event for logging
func (qe *QuestEvent) Summary() string {
	return fmt.Sprintf("Quest %s: %s (%s)", qe.QuestID, qe.Status.GetDescription(), qe.Status.String())
}

// extractCompleteJSON extracts a complete JSON object by matching braces
func extractCompleteJSON(jsonString string) string {
	braceCount := 0
	var result strings.Builder
	
	for _, char := range jsonString {
		result.WriteRune(char)
		
		if char == '{' {
			braceCount++
		} else if char == '}' {
			braceCount--
			if braceCount == 0 {
				// Found the complete JSON object
				return result.String()
			}
		}
	}
	
	// If we get here, the braces didn't match properly
	return ""
}