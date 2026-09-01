package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tarkov-screenshot-analyzer/party-server/internal/hub"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// LogEntry represents a server log entry
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// logWriter captures log output and forwards to WebUI
type logWriter struct {
	webUI  *WebUI
	stdout io.Writer
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	// Write to stdout as normal
	n, err = lw.stdout.Write(p)

	// Also send to WebUI
	msg := strings.TrimSpace(string(p))
	if msg != "" && lw.webUI != nil {
		// Parse log level from message
		level := "info"
		if strings.Contains(msg, "error") || strings.Contains(msg, "Error") || strings.Contains(msg, "ERROR") {
			level = "error"
		} else if strings.Contains(msg, "warn") || strings.Contains(msg, "Warn") || strings.Contains(msg, "WARN") {
			level = "warn"
		}

		// Strip timestamp from Go log (it adds its own)
		// Format is usually: 2024/01/15 14:30:25 message
		if len(msg) > 20 && msg[4] == '/' && msg[7] == '/' {
			if idx := strings.Index(msg, " "); idx > 0 {
				if idx2 := strings.Index(msg[idx+1:], " "); idx2 > 0 {
					msg = msg[idx+1+idx2+1:]
				}
			}
		}

		lw.webUI.AddLog(level, msg)
	}

	return
}

// WebUI provides a web-based console interface
type WebUI struct {
	hub      *hub.Hub
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	logs     []LogEntry
	logMu    sync.RWMutex
}

// NewWebUI creates a new web UI instance
func NewWebUI(h *hub.Hub) *WebUI {
	w := &WebUI{
		hub:     h,
		clients: make(map[*websocket.Conn]bool),
		logs:    make([]LogEntry, 0, 500), // Increased buffer for server logs
	}

	// Capture all log output and forward to WebUI
	log.SetOutput(&logWriter{webUI: w, stdout: os.Stdout})

	// Start status broadcaster
	go w.broadcastLoop()

	return w
}

// RegisterRoutes registers web UI routes
func (w *WebUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", w.handleIndex)
	mux.HandleFunc("/console", w.handleIndex)
	mux.HandleFunc("/ws/console", w.handleWebSocket)
	mux.HandleFunc("/api/command", w.handleCommand)
	mux.HandleFunc("/api/status", w.handleStatus)
	mux.HandleFunc("/api/autocomplete", w.handleAutocomplete)

	log.Println("[WebUI] Web console registered")
}

// AddLog adds a log entry and broadcasts to clients
func (w *WebUI) AddLog(level, message string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Level:   level,
		Message: message,
	}

	w.logMu.Lock()
	w.logs = append(w.logs, entry)
	if len(w.logs) > 100 {
		w.logs = w.logs[1:]
	}
	w.logMu.Unlock()

	w.broadcastLog(entry)
}

func (w *WebUI) handleIndex(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html")
	rw.Write([]byte(indexHTML))
}

func (w *WebUI) handleWebSocket(rw http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		log.Printf("[WebUI] WebSocket upgrade error: %v", err)
		return
	}

	w.mu.Lock()
	w.clients[conn] = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.clients, conn)
		w.mu.Unlock()
		conn.Close()
	}()

	// Send initial status and logs
	w.sendFullStatus(conn)

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (w *WebUI) handleStatus(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")

	stats := w.hub.GetStats()
	clients := w.hub.GetAllClients()
	parties := w.hub.GetAllParties()
	friendRequests := w.hub.GetAllPendingFriendRequests()

	json.NewEncoder(rw).Encode(map[string]interface{}{
		"stats":          stats,
		"clients":        clients,
		"parties":        parties,
		"friendRequests": friendRequests,
	})
}

func (w *WebUI) handleAutocomplete(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	parts := strings.Fields(query)

	var suggestions []string

	if len(parts) == 0 {
		suggestions = []string{"create-client", "remove-client", "cleanup", "create-party", "join-party", "delete-party", "add-friend", "remove-friend", "send-position"}
	} else if len(parts) == 1 {
		// Command autocomplete
		commands := []string{"create-client", "remove-client", "cleanup", "create-party", "join-party", "delete-party", "add-friend", "remove-friend", "send-position", "help", "clear"}
		prefix := strings.ToLower(parts[0])
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, prefix) {
				suggestions = append(suggestions, cmd)
			}
		}
	} else {
		// Argument autocomplete - suggest client IDs
		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "remove-client", "create-party", "join-party", "add-friend", "remove-friend", "send-position":
			clients := w.hub.GetAllClients()
			prefix := ""
			if len(parts) > 1 {
				prefix = strings.ToLower(parts[len(parts)-1])
			}
			for _, c := range clients {
				shortID := c.ID[:12]
				if prefix == "" || strings.HasPrefix(strings.ToLower(shortID), prefix) || strings.HasPrefix(strings.ToLower(c.DisplayName), prefix) {
					suggestions = append(suggestions, shortID+" ("+c.DisplayName+")")
				}
			}
		case "delete-party":
			parties := w.hub.GetAllParties()
			prefix := ""
			if len(parts) > 1 {
				prefix = strings.ToUpper(parts[len(parts)-1])
			}
			for _, p := range parties {
				if prefix == "" || strings.HasPrefix(p.Code, prefix) {
					suggestions = append(suggestions, p.Code)
				}
			}
		}
	}

	json.NewEncoder(rw).Encode(suggestions)
}

func (w *WebUI) handleCommand(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request", http.StatusBadRequest)
		return
	}

	result := w.executeCommand(req.Command, req.Args)

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(result)

	// Broadcast status update
	w.broadcastStatus()
}

func (w *WebUI) executeCommand(cmd string, args []string) map[string]interface{} {
	switch cmd {
	case "create-client":
		name := ""
		if len(args) > 0 {
			name = strings.Join(args, " ")
		}
		client, err := w.hub.CreateTestClient(name, "", "")
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Created test client: %s (%s)", client.DisplayName, client.ID[:12]))
		return map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Created client: %s", client.DisplayName),
			"data":    map[string]string{"clientId": client.ID, "displayName": client.DisplayName},
		}

	case "remove-client":
		if len(args) < 1 {
			return map[string]interface{}{"success": false, "error": "Client ID required"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		if clientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		if err := w.hub.RemoveTestClient(clientID); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Removed client: %s", clientID[:12]))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Removed client: %s", clientID[:12])}

	case "cleanup":
		count := w.hub.CleanupTestClients()
		w.AddLog("info", fmt.Sprintf("Cleaned up %d test clients", count))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Cleaned up %d test clients", count)}

	case "create-party":
		if len(args) < 1 {
			return map[string]interface{}{"success": false, "error": "Client ID required"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		if clientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		code, err := w.hub.CreateTestParty(clientID, nil)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Created party: %s", code))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Created party: %s", code), "data": map[string]string{"partyCode": code}}

	case "join-party":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Client ID and party code required"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		if clientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		if err := w.hub.JoinTestParty(clientID, strings.ToUpper(args[1])); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Client %s joined party %s", clientID[:8], args[1]))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Joined party: %s", args[1])}

	case "leave-party":
		if len(args) < 1 {
			return map[string]interface{}{"success": false, "error": "Client ID required"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		if clientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		if err := w.hub.LeaveTestParty(clientID); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Client %s left party", clientID[:8]))
		return map[string]interface{}{"success": true, "message": "Left party"}

	case "delete-party":
		if len(args) < 1 {
			return map[string]interface{}{"success": false, "error": "Party code required"}
		}
		if err := w.hub.DeleteParty(strings.ToUpper(args[0])); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Deleted party: %s", args[0]))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Deleted party: %s", args[0])}

	case "add-friend":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Two client IDs required"}
		}
		clientID1 := w.hub.FindClientByPrefix(args[0])
		clientID2 := w.hub.FindClientByPrefix(args[1])
		if clientID1 == "" || clientID2 == "" {
			return map[string]interface{}{"success": false, "error": "One or both clients not found"}
		}
		if err := w.hub.CreateTestFriendship(clientID1, clientID2); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Added friendship: %s <-> %s", clientID1[:8], clientID2[:8]))
		return map[string]interface{}{"success": true, "message": "Friendship added"}

	case "remove-friend":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Two client IDs required"}
		}
		clientID1 := w.hub.FindClientByPrefix(args[0])
		clientID2 := w.hub.FindClientByPrefix(args[1])
		if clientID1 == "" || clientID2 == "" {
			return map[string]interface{}{"success": false, "error": "One or both clients not found"}
		}
		if err := w.hub.RemoveTestFriendship(clientID1, clientID2); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Removed friendship: %s <-> %s", clientID1[:8], clientID2[:8]))
		return map[string]interface{}{"success": true, "message": "Friendship removed"}

	case "send-request":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Usage: send-request <fromTestClientId> <toClientId>"}
		}
		fromClientID := w.hub.FindClientByPrefix(args[0])
		toClientID := w.hub.FindClientByPrefix(args[1])
		if fromClientID == "" || toClientID == "" {
			return map[string]interface{}{"success": false, "error": "One or both clients not found"}
		}
		if err := w.hub.SendFriendRequestForClient(fromClientID, toClientID); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Friend request sent: %s -> %s", fromClientID[:8], toClientID[:8]))
		return map[string]interface{}{"success": true, "message": "Friend request sent"}

	case "send-position":
		if len(args) < 5 {
			return map[string]interface{}{"success": false, "error": "Usage: send-position <clientId> <map> <x> <y> <z>"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		if clientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		var x, y, z float64
		fmt.Sscanf(args[2], "%f", &x)
		fmt.Sscanf(args[3], "%f", &y)
		fmt.Sscanf(args[4], "%f", &z)
		if err := w.hub.SendTestPosition(clientID, args[1], x, y, z, 0); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Position sent: %s @ %s (%.0f, %.0f, %.0f)", clientID[:8], args[1], x, y, z))
		return map[string]interface{}{"success": true, "message": fmt.Sprintf("Position sent")}

	case "accept-request":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Usage: accept-request <testClientId> <fromClientId>"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		fromClientID := w.hub.FindClientByPrefix(args[1])
		if clientID == "" || fromClientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		if err := w.hub.AcceptFriendRequestForClient(clientID, fromClientID); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Friend request accepted: %s accepted %s", clientID[:8], fromClientID[:8]))
		return map[string]interface{}{"success": true, "message": "Friend request accepted"}

	case "decline-request":
		if len(args) < 2 {
			return map[string]interface{}{"success": false, "error": "Usage: decline-request <testClientId> <fromClientId>"}
		}
		clientID := w.hub.FindClientByPrefix(args[0])
		fromClientID := w.hub.FindClientByPrefix(args[1])
		if clientID == "" || fromClientID == "" {
			return map[string]interface{}{"success": false, "error": "Client not found"}
		}
		if err := w.hub.DeclineFriendRequestForClient(clientID, fromClientID); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		w.AddLog("info", fmt.Sprintf("Friend request declined: %s declined %s", clientID[:8], fromClientID[:8]))
		return map[string]interface{}{"success": true, "message": "Friend request declined"}

	default:
		return map[string]interface{}{"success": false, "error": "Unknown command: " + cmd}
	}
}

func (w *WebUI) sendFullStatus(conn *websocket.Conn) {
	stats := w.hub.GetStats()
	clients := w.hub.GetAllClients()
	parties := w.hub.GetAllParties()
	friendRequests := w.hub.GetAllPendingFriendRequests()

	w.logMu.RLock()
	logs := make([]LogEntry, len(w.logs))
	copy(logs, w.logs)
	w.logMu.RUnlock()

	msg, _ := json.Marshal(map[string]interface{}{
		"type":           "init",
		"stats":          stats,
		"clients":        clients,
		"parties":        parties,
		"friendRequests": friendRequests,
		"logs":           logs,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func (w *WebUI) broadcastStatus() {
	w.mu.RLock()
	defer w.mu.RUnlock()

	stats := w.hub.GetStats()
	clients := w.hub.GetAllClients()
	parties := w.hub.GetAllParties()
	friendRequests := w.hub.GetAllPendingFriendRequests()

	msg, _ := json.Marshal(map[string]interface{}{
		"type":           "status",
		"stats":          stats,
		"clients":        clients,
		"parties":        parties,
		"friendRequests": friendRequests,
	})

	for conn := range w.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (w *WebUI) broadcastLog(entry LogEntry) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	msg, _ := json.Marshal(map[string]interface{}{
		"type": "log",
		"log":  entry,
	})

	for conn := range w.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (w *WebUI) broadcastLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		w.broadcastStatus()
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Party Server Console</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        :root {
            --primary-purple: #5865F2;
            --dark-purple: #4752C4;
            --light-purple: #7289DA;
            --electric-purple: #8B5FBF;
            --neon-green: #00D4AA;
            --warning: #FEE75C;
            --error: #ED4245;
            --bg-dark: #0C0E14;
            --bg-darker: #06080B;
            --bg-card: #1A1D26;
            --bg-hover: #232631;
            --text-primary: #FFFFFF;
            --text-secondary: #B9BBBE;
            --text-muted: #72767D;
            --border-color: #2A2D38;
        }

        body {
            font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
            background: var(--bg-dark);
            color: var(--text-primary);
            height: 100vh;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        /* Header */
        .header {
            background: var(--bg-card);
            padding: 12px 20px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .header-left {
            display: flex;
            align-items: center;
            gap: 15px;
        }

        .logo {
            font-size: 1.3em;
            font-weight: 700;
            background: linear-gradient(135deg, var(--primary-purple), var(--neon-green));
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .stats {
            display: flex;
            gap: 15px;
        }

        .stat {
            background: var(--bg-hover);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 0.85em;
            border: 1px solid var(--border-color);
        }

        .stat-value {
            color: var(--neon-green);
            font-weight: 600;
        }

        .stat-value.real {
            color: var(--light-purple);
        }

        .toggle-group {
            display: flex;
            gap: 15px;
        }

        .toggle-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 0.85em;
            color: var(--text-secondary);
            cursor: pointer;
        }

        .toggle-item input {
            width: 16px;
            height: 16px;
            accent-color: var(--primary-purple);
        }

        /* Main Layout */
        .main {
            display: flex;
            flex: 1;
            overflow: hidden;
        }

        /* Sidebar */
        .sidebar {
            width: 280px;
            background: var(--bg-card);
            border-right: 1px solid var(--border-color);
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        .sidebar-section {
            display: flex;
            flex-direction: column;
            overflow: hidden;
            min-height: 80px;
        }

        .sidebar-section.flex-1 {
            flex: 1;
        }

        .sidebar-header {
            background: var(--bg-hover);
            padding: 10px 15px;
            font-weight: 600;
            font-size: 0.85em;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .sidebar-header .count {
            background: var(--bg-dark);
            padding: 2px 8px;
            border-radius: 10px;
            font-size: 0.85em;
        }

        .sidebar-content {
            flex: 1;
            overflow-y: auto;
            padding: 8px;
        }

        .client-card {
            background: var(--bg-dark);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 10px;
            margin-bottom: 8px;
            transition: all 0.2s;
        }

        .client-card.selectable {
            cursor: pointer;
        }

        .client-card.selectable:hover {
            border-color: var(--primary-purple);
            transform: translateY(-1px);
        }

        .client-card.selected {
            border-color: var(--neon-green);
            box-shadow: 0 0 10px rgba(0, 212, 170, 0.2);
        }

        .client-card.test {
            border-left: 3px solid var(--primary-purple);
        }

        .client-card.real {
            border-left: 3px solid var(--neon-green);
            opacity: 0.8;
        }

        .client-name {
            font-weight: 600;
            margin-bottom: 4px;
        }

        .client-id {
            font-size: 0.75em;
            color: var(--text-muted);
            font-family: monospace;
        }

        .client-status {
            display: flex;
            gap: 8px;
            margin-top: 6px;
            flex-wrap: wrap;
        }

        .client-badge {
            font-size: 0.7em;
            padding: 2px 6px;
            border-radius: 4px;
            background: var(--bg-hover);
            color: var(--text-secondary);
        }

        .client-badge.party {
            background: rgba(88, 101, 242, 0.2);
            color: var(--primary-purple);
        }

        .client-badge.test {
            background: rgba(139, 95, 191, 0.2);
            color: var(--electric-purple);
        }

        .client-badge.real {
            background: rgba(0, 212, 170, 0.2);
            color: var(--neon-green);
        }

        .party-card {
            background: var(--bg-dark);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 10px;
            margin-bottom: 8px;
        }

        .party-code {
            font-weight: 700;
            color: var(--neon-green);
            font-family: monospace;
            font-size: 1.1em;
        }

        .party-members {
            font-size: 0.8em;
            color: var(--text-muted);
            margin-top: 4px;
        }

        .party-member-list {
            font-size: 0.75em;
            color: var(--text-secondary);
            margin-top: 6px;
            padding-left: 10px;
            border-left: 2px solid var(--border-color);
        }

        .party-member-list .member {
            padding: 2px 0;
        }

        .party-member-list .member.test {
            color: var(--electric-purple);
        }

        .party-member-list .member.real {
            color: var(--neon-green);
        }

        .request-card {
            background: var(--bg-dark);
            border: 1px solid var(--warning);
            border-left: 3px solid var(--warning);
            border-radius: 8px;
            padding: 10px;
            margin-bottom: 8px;
        }

        .request-info {
            font-size: 0.85em;
            margin-bottom: 8px;
        }

        .request-from {
            color: var(--text-primary);
            font-weight: 600;
        }

        .request-to {
            color: var(--neon-green);
        }

        .request-arrow {
            color: var(--text-muted);
            margin: 0 6px;
        }

        .request-time {
            font-size: 0.75em;
            color: var(--text-muted);
        }

        .request-actions {
            display: flex;
            gap: 6px;
        }

        .request-actions button {
            flex: 1;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.75em;
            cursor: pointer;
            border: 1px solid var(--border-color);
            background: var(--bg-hover);
            color: var(--text-primary);
        }

        .request-actions button.accept {
            background: rgba(0, 212, 170, 0.2);
            border-color: var(--neon-green);
            color: var(--neon-green);
        }

        .request-actions button.decline {
            background: rgba(237, 66, 69, 0.2);
            border-color: var(--error);
            color: var(--error);
        }

        .request-actions button:hover {
            opacity: 0.8;
        }

        .empty-state {
            text-align: center;
            padding: 20px;
            color: var(--text-muted);
            font-size: 0.9em;
        }

        /* Console Area */
        .console-area {
            flex: 1;
            display: flex;
            flex-direction: column;
            padding: 15px;
            overflow: hidden;
        }

        /* Bot Control Panel */
        .control-panel {
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 10px;
            padding: 15px;
            margin-bottom: 15px;
        }

        .control-panel-header {
            font-weight: 600;
            margin-bottom: 12px;
            color: var(--text-secondary);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .selected-info {
            font-size: 0.85em;
            color: var(--neon-green);
        }

        .control-sections {
            display: flex;
            flex-wrap: wrap;
            gap: 15px;
        }

        .control-group {
            background: var(--bg-dark);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 10px;
        }

        .control-group-label {
            font-size: 0.7em;
            text-transform: uppercase;
            color: var(--text-muted);
            margin-bottom: 8px;
            letter-spacing: 0.5px;
        }

        .control-group-buttons {
            display: flex;
            flex-wrap: wrap;
            gap: 6px;
        }

        .control-buttons {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
        }

        .ctrl-btn {
            background: var(--bg-hover);
            border: 1px solid var(--border-color);
            padding: 8px 14px;
            border-radius: 6px;
            color: var(--text-primary);
            font-size: 0.85em;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .ctrl-btn:hover:not(:disabled) {
            border-color: var(--primary-purple);
            background: rgba(88, 101, 242, 0.1);
        }

        .ctrl-btn.primary {
            background: var(--primary-purple);
            border-color: var(--primary-purple);
        }

        .ctrl-btn.primary:hover:not(:disabled) {
            background: var(--dark-purple);
        }

        .ctrl-btn.success {
            background: rgba(0, 212, 170, 0.15);
            border-color: var(--neon-green);
            color: var(--neon-green);
        }

        .ctrl-btn.danger {
            background: rgba(237, 66, 69, 0.15);
            border-color: var(--error);
            color: var(--error);
        }

        .ctrl-btn:disabled {
            opacity: 0.4;
            cursor: not-allowed;
        }

        /* Output Area */
        .output-container {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: var(--bg-darker);
            border: 1px solid var(--border-color);
            border-radius: 10px;
            overflow: hidden;
        }

        .output {
            flex: 1;
            padding: 12px;
            overflow-y: auto;
            font-family: 'Consolas', 'Monaco', monospace;
            font-size: 0.85em;
            line-height: 1.5;
        }

        .output-line {
            margin-bottom: 4px;
            padding: 2px 0;
        }

        .output-line.hidden {
            display: none;
        }

        .output-line.success {
            color: var(--neon-green);
        }

        .output-line.error {
            color: var(--error);
        }

        .output-line.info {
            color: var(--light-purple);
        }

        .output-line.log {
            color: var(--text-muted);
        }

        .output-line.command {
            color: var(--primary-purple);
        }

        .output-line .timestamp {
            color: var(--text-muted);
            margin-right: 8px;
        }

        .output-line .level {
            display: inline-block;
            width: 50px;
            text-transform: uppercase;
            font-size: 0.8em;
        }

        /* Input Area */
        .input-container {
            border-top: 1px solid var(--border-color);
            padding: 10px;
            display: flex;
            gap: 10px;
            position: relative;
        }

        .command-input {
            flex: 1;
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 10px 14px;
            color: var(--text-primary);
            font-family: 'Consolas', 'Monaco', monospace;
            font-size: 0.9em;
        }

        .command-input:focus {
            outline: none;
            border-color: var(--primary-purple);
            box-shadow: 0 0 0 2px rgba(88, 101, 242, 0.2);
        }

        .send-btn {
            background: var(--primary-purple);
            border: none;
            padding: 10px 20px;
            border-radius: 6px;
            color: white;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }

        .send-btn:hover {
            background: var(--dark-purple);
        }

        /* Autocomplete */
        .autocomplete {
            position: absolute;
            bottom: 100%;
            left: 10px;
            right: 80px;
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            max-height: 200px;
            overflow-y: auto;
            display: none;
            z-index: 100;
        }

        .autocomplete.show {
            display: block;
        }

        .autocomplete-item {
            padding: 8px 12px;
            cursor: pointer;
            font-family: monospace;
            font-size: 0.85em;
        }

        .autocomplete-item:hover,
        .autocomplete-item.selected {
            background: var(--bg-hover);
        }

        /* Modal */
        .modal-overlay {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.7);
            display: none;
            align-items: center;
            justify-content: center;
            z-index: 1000;
        }

        .modal-overlay.show {
            display: flex;
        }

        .modal {
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 20px;
            min-width: 300px;
            max-width: 400px;
        }

        .modal-title {
            font-weight: 600;
            margin-bottom: 15px;
            font-size: 1.1em;
        }

        .modal-input {
            width: 100%;
            background: var(--bg-dark);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 10px;
            color: var(--text-primary);
            margin-bottom: 15px;
        }

        .modal-input:focus {
            outline: none;
            border-color: var(--primary-purple);
        }

        .modal-buttons {
            display: flex;
            gap: 10px;
            justify-content: flex-end;
        }

        /* Scrollbar */
        ::-webkit-scrollbar {
            width: 8px;
            height: 8px;
        }

        ::-webkit-scrollbar-track {
            background: var(--bg-darker);
        }

        ::-webkit-scrollbar-thumb {
            background: var(--border-color);
            border-radius: 4px;
        }

        ::-webkit-scrollbar-thumb:hover {
            background: var(--primary-purple);
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="header-left">
            <div class="logo">Party Server Console</div>
            <div class="stats">
                <div class="stat">Real: <span class="stat-value real" id="realCount">0</span></div>
                <div class="stat">Test: <span class="stat-value" id="testCount">0</span></div>
                <div class="stat">Parties: <span class="stat-value" id="partyCount">0</span></div>
            </div>
        </div>
        <div class="toggle-group">
            <label class="toggle-item">
                <input type="checkbox" id="showCommands" checked>
                <span>Commands</span>
            </label>
            <label class="toggle-item">
                <input type="checkbox" id="showLogs" checked>
                <span>Server Logs</span>
            </label>
        </div>
    </div>

    <div class="main">
        <div class="sidebar">
            <div class="sidebar-section flex-1">
                <div class="sidebar-header">
                    <span>Test Clients</span>
                    <span class="count" id="testClientCount">0</span>
                </div>
                <div class="sidebar-content" id="testClientList">
                    <div class="empty-state">No test clients</div>
                </div>
            </div>
            <div class="sidebar-section" style="max-height: 30%;">
                <div class="sidebar-header">
                    <span>Real Clients</span>
                    <span class="count" id="realClientCount">0</span>
                </div>
                <div class="sidebar-content" id="realClientList">
                    <div class="empty-state">No real clients</div>
                </div>
            </div>
            <div class="sidebar-section" style="max-height: 25%;">
                <div class="sidebar-header">
                    <span>Active Parties</span>
                    <span class="count" id="partyListCount">0</span>
                </div>
                <div class="sidebar-content" id="partyList">
                    <div class="empty-state">No parties</div>
                </div>
            </div>
            <div class="sidebar-section" style="max-height: 25%;">
                <div class="sidebar-header" style="background: rgba(254, 231, 92, 0.1);">
                    <span>Friend Requests</span>
                    <span class="count" id="requestCount" style="background: var(--warning); color: #000;">0</span>
                </div>
                <div class="sidebar-content" id="requestList">
                    <div class="empty-state">No pending requests</div>
                </div>
            </div>
        </div>

        <div class="console-area">
            <div class="control-panel">
                <div class="control-panel-header">
                    <span>Test Bot Controls</span>
                    <span class="selected-info" id="selectedInfo">Select a test client from sidebar</span>
                </div>
                <div class="control-sections">
                    <div class="control-group">
                        <div class="control-group-label">Bot Management</div>
                        <div class="control-group-buttons">
                            <button class="ctrl-btn primary" onclick="showCreateClientModal()">+ Create Bot</button>
                            <button class="ctrl-btn danger" id="btnRemove" onclick="removeSelected()" disabled>Delete Bot</button>
                            <button class="ctrl-btn danger" onclick="cleanup()">Delete All Bots</button>
                        </div>
                    </div>
                    <div class="control-group">
                        <div class="control-group-label">Party Actions (Bot joins/creates parties)</div>
                        <div class="control-group-buttons">
                            <button class="ctrl-btn success" id="btnCreateParty" onclick="createPartyForSelected()" disabled>Bot Creates Party</button>
                            <button class="ctrl-btn" id="btnJoinParty" onclick="showJoinPartyModal()" disabled>Bot Joins Party</button>
                            <button class="ctrl-btn" id="btnLeaveParty" onclick="leavePartyForSelected()" disabled>Bot Leaves Party</button>
                        </div>
                    </div>
                    <div class="control-group">
                        <div class="control-group-label">Friend Actions (Bot sends/adds friends)</div>
                        <div class="control-group-buttons">
                            <button class="ctrl-btn" id="btnSendRequest" onclick="showSendRequestModal()" disabled>Bot Sends Friend Request</button>
                            <button class="ctrl-btn" id="btnAddFriend" onclick="showAddFriendModal()" disabled>Force Friendship (Skip Request)</button>
                        </div>
                    </div>
                    <div class="control-group">
                        <div class="control-group-label">Position (Bot broadcasts location)</div>
                        <div class="control-group-buttons">
                            <button class="ctrl-btn" id="btnSendPos" onclick="showPositionModal()" disabled>Bot Sends Position</button>
                        </div>
                    </div>
                </div>
            </div>

            <div class="output-container">
                <div class="output" id="output">
                    <div class="output-line info" data-type="command">
                        <span class="timestamp">[--:--:--]</span>
                        Welcome to Party Server Console (Test clients only)
                    </div>
                </div>
                <div class="input-container">
                    <div class="autocomplete" id="autocomplete"></div>
                    <input type="text" class="command-input" id="commandInput"
                           placeholder="Type command... (Tab for autocomplete)"
                           onkeydown="handleInputKeydown(event)"
                           oninput="handleInputChange()">
                    <button class="send-btn" onclick="sendCommand()">Send</button>
                </div>
            </div>
        </div>
    </div>

    <!-- Create Client Modal -->
    <div class="modal-overlay" id="createClientModal">
        <div class="modal">
            <div class="modal-title">Create Test Client</div>
            <input type="text" class="modal-input" id="newClientName" placeholder="Display name (optional)">
            <div class="modal-buttons">
                <button class="ctrl-btn" onclick="hideModals()">Cancel</button>
                <button class="ctrl-btn primary" onclick="createClient()">Create</button>
            </div>
        </div>
    </div>

    <!-- Join Party Modal -->
    <div class="modal-overlay" id="joinPartyModal">
        <div class="modal">
            <div class="modal-title">Join Party</div>
            <input type="text" class="modal-input" id="partyCodeInput" placeholder="Party code (e.g., ALPHA-1234)">
            <div class="modal-buttons">
                <button class="ctrl-btn" onclick="hideModals()">Cancel</button>
                <button class="ctrl-btn primary" onclick="joinParty()">Join</button>
            </div>
        </div>
    </div>

    <!-- Add Friend Modal -->
    <div class="modal-overlay" id="addFriendModal">
        <div class="modal">
            <div class="modal-title">Add Friend</div>
            <select class="modal-input" id="friendSelect"></select>
            <div class="modal-buttons">
                <button class="ctrl-btn" onclick="hideModals()">Cancel</button>
                <button class="ctrl-btn primary" onclick="addFriend()">Add</button>
            </div>
        </div>
    </div>

    <!-- Send Friend Request Modal -->
    <div class="modal-overlay" id="sendRequestModal">
        <div class="modal">
            <div class="modal-title">Send Friend Request</div>
            <p style="font-size: 0.85em; color: var(--text-muted); margin-bottom: 10px;">Send a friend request that the other user must accept</p>
            <select class="modal-input" id="requestTargetSelect"></select>
            <div class="modal-buttons">
                <button class="ctrl-btn" onclick="hideModals()">Cancel</button>
                <button class="ctrl-btn primary" onclick="sendFriendRequest()">Send Request</button>
            </div>
        </div>
    </div>

    <!-- Position Modal -->
    <div class="modal-overlay" id="positionModal">
        <div class="modal">
            <div class="modal-title">Send Position</div>
            <select class="modal-input" id="mapSelect">
                <option value="customs">Customs</option>
                <option value="woods">Woods</option>
                <option value="shoreline">Shoreline</option>
                <option value="interchange">Interchange</option>
                <option value="reserve">Reserve</option>
                <option value="lighthouse">Lighthouse</option>
                <option value="streets">Streets of Tarkov</option>
                <option value="labs">The Lab</option>
                <option value="factory">Factory</option>
                <option value="ground-zero">Ground Zero</option>
            </select>
            <input type="number" class="modal-input" id="posX" placeholder="X coordinate" value="0">
            <input type="number" class="modal-input" id="posY" placeholder="Y coordinate" value="0">
            <input type="number" class="modal-input" id="posZ" placeholder="Z coordinate" value="0">
            <div class="modal-buttons">
                <button class="ctrl-btn" onclick="hideModals()">Cancel</button>
                <button class="ctrl-btn primary" onclick="sendPosition()">Send</button>
            </div>
        </div>
    </div>

    <script>
        let ws;
        let clients = [];
        let parties = [];
        let selectedClientId = null;
        let autocompleteIndex = -1;
        let autocompleteSuggestions = [];

        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws/console');

            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);

                if (data.type === 'init') {
                    updateStatus(data);
                    if (data.logs) {
                        data.logs.forEach(log => addLogLine(log));
                    }
                } else if (data.type === 'status') {
                    updateStatus(data);
                } else if (data.type === 'log') {
                    addLogLine(data.log);
                }
            };

            ws.onclose = function() {
                setTimeout(connect, 2000);
            };
        }

        function updateStatus(data) {
            const testClients = (data.clients || []).filter(c => c.isTest);
            const realClients = (data.clients || []).filter(c => !c.isTest);
            const friendRequests = data.friendRequests || [];

            document.getElementById('realCount').textContent = realClients.length;
            document.getElementById('testCount').textContent = testClients.length;
            document.getElementById('partyCount').textContent = data.stats.parties || 0;
            document.getElementById('testClientCount').textContent = testClients.length;
            document.getElementById('realClientCount').textContent = realClients.length;
            document.getElementById('partyListCount').textContent = (data.parties || []).length;
            document.getElementById('requestCount').textContent = friendRequests.length;

            clients = data.clients || [];
            parties = data.parties || [];

            updateClientLists(testClients, realClients);
            updatePartyList();
            updateFriendRequestList(friendRequests);
            updateButtonStates();
        }

        function updateClientLists(testClients, realClients) {
            // Test clients list (selectable)
            const testList = document.getElementById('testClientList');
            if (testClients.length === 0) {
                testList.innerHTML = '<div class="empty-state">No test clients</div>';
            } else {
                testList.innerHTML = testClients.map(c => {
                    const isSelected = c.clientId === selectedClientId;
                    const badges = ['<span class="client-badge test">TEST</span>'];
                    if (c.partyCode) badges.push('<span class="client-badge party">' + c.partyCode + '</span>');

                    return '<div class="client-card test selectable' + (isSelected ? ' selected' : '') + '" onclick="selectClient(\'' + c.clientId + '\')">' +
                        '<div class="client-name">' + escapeHtml(c.displayName) + '</div>' +
                        '<div class="client-id">' + c.clientId.substring(0, 16) + '...</div>' +
                        '<div class="client-status">' + badges.join('') + '</div>' +
                        '</div>';
                }).join('');
            }

            // Real clients list (not selectable, just for visibility)
            const realList = document.getElementById('realClientList');
            if (realClients.length === 0) {
                realList.innerHTML = '<div class="empty-state">No real clients connected</div>';
            } else {
                realList.innerHTML = realClients.map(c => {
                    const badges = ['<span class="client-badge real">REAL</span>'];
                    if (c.partyCode) badges.push('<span class="client-badge party">' + c.partyCode + '</span>');

                    return '<div class="client-card real">' +
                        '<div class="client-name">' + escapeHtml(c.displayName) + '</div>' +
                        '<div class="client-id">' + c.clientId.substring(0, 16) + '...</div>' +
                        '<div class="client-status">' + badges.join('') + '</div>' +
                        '</div>';
                }).join('');
            }
        }

        function updatePartyList() {
            const list = document.getElementById('partyList');
            if (parties.length === 0) {
                list.innerHTML = '<div class="empty-state">No parties</div>';
                return;
            }

            list.innerHTML = parties.map(p => {
                const memberHtml = p.members ? p.members.map(m =>
                    '<div class="member ' + (m.isTest ? 'test' : 'real') + '">' +
                    (m.isTest ? '[T] ' : '[R] ') + escapeHtml(m.displayName) +
                    '</div>'
                ).join('') : '';

                return '<div class="party-card">' +
                    '<div class="party-code">' + p.code + '</div>' +
                    '<div class="party-members">' + p.memberCount + ' member(s)</div>' +
                    (memberHtml ? '<div class="party-member-list">' + memberHtml + '</div>' : '') +
                    '</div>';
            }).join('');
        }

        function updateFriendRequestList(requests) {
            const list = document.getElementById('requestList');
            if (!requests || requests.length === 0) {
                list.innerHTML = '<div class="empty-state">No pending requests</div>';
                return;
            }

            list.innerHTML = requests.map(r => {
                // Only show accept/decline if the recipient is a test client
                const canRespond = r.toIsTest;
                const actionsHtml = canRespond ?
                    '<div class="request-actions">' +
                    '<button class="accept" onclick="acceptRequest(\'' + r.toClientId + '\', \'' + r.fromClientId + '\')">Accept</button>' +
                    '<button class="decline" onclick="declineRequest(\'' + r.toClientId + '\', \'' + r.fromClientId + '\')">Decline</button>' +
                    '</div>' : '';

                return '<div class="request-card">' +
                    '<div class="request-info">' +
                    '<span class="request-from">' + (r.fromIsTest ? '[T] ' : '[R] ') + escapeHtml(r.fromName) + '</span>' +
                    '<span class="request-arrow">→</span>' +
                    '<span class="request-to">' + (r.toIsTest ? '[T] ' : '[R] ') + escapeHtml(r.toName) + '</span>' +
                    '</div>' +
                    '<div class="request-time">Sent at ' + r.sentAt + '</div>' +
                    actionsHtml +
                    '</div>';
            }).join('');
        }

        function acceptRequest(toClientId, fromClientId) {
            runCommand('accept-request', [toClientId, fromClientId]);
        }

        function declineRequest(toClientId, fromClientId) {
            runCommand('decline-request', [toClientId, fromClientId]);
        }

        function selectClient(clientId) {
            // Only allow selecting test clients
            const client = clients.find(c => c.clientId === clientId);
            if (!client || !client.isTest) return;

            selectedClientId = selectedClientId === clientId ? null : clientId;
            updateClientLists(
                clients.filter(c => c.isTest),
                clients.filter(c => !c.isTest)
            );
            updateButtonStates();
        }

        function updateButtonStates() {
            const selectedClient = clients.find(c => c.clientId === selectedClientId);
            const hasSelection = selectedClient && selectedClient.isTest;
            const inParty = selectedClient && selectedClient.partyCode;
            const testClients = clients.filter(c => c.isTest);

            // Update selected info with more context
            const selectedInfo = document.getElementById('selectedInfo');
            if (hasSelection) {
                let status = 'Bot: ' + selectedClient.displayName;
                if (inParty) {
                    status += ' (in party ' + selectedClient.partyCode + ')';
                }
                selectedInfo.textContent = status;
                selectedInfo.style.color = 'var(--neon-green)';
            } else if (testClients.length === 0) {
                selectedInfo.textContent = 'No bots - click "+ Create Bot" to start';
                selectedInfo.style.color = 'var(--warning)';
            } else {
                selectedInfo.textContent = 'Click a bot in "Test Clients" sidebar to select';
                selectedInfo.style.color = 'var(--text-muted)';
            }

            document.getElementById('btnCreateParty').disabled = !hasSelection || inParty;
            document.getElementById('btnJoinParty').disabled = !hasSelection || inParty;
            document.getElementById('btnLeaveParty').disabled = !hasSelection || !inParty;
            document.getElementById('btnSendRequest').disabled = !hasSelection || clients.length < 2;
            document.getElementById('btnAddFriend').disabled = !hasSelection || clients.length < 2;
            document.getElementById('btnSendPos').disabled = !hasSelection || !inParty;
            document.getElementById('btnRemove').disabled = !hasSelection;
        }

        function showCreateClientModal() {
            document.getElementById('createClientModal').classList.add('show');
            document.getElementById('newClientName').focus();
        }

        function showJoinPartyModal() {
            document.getElementById('joinPartyModal').classList.add('show');
            document.getElementById('partyCodeInput').focus();
        }

        function showSendRequestModal() {
            const select = document.getElementById('requestTargetSelect');
            // Show all clients (test and real) as potential targets
            select.innerHTML = clients
                .filter(c => c.clientId !== selectedClientId)
                .map(c => '<option value="' + c.clientId + '">' + (c.isTest ? '[T] ' : '[R] ') + escapeHtml(c.displayName) + '</option>')
                .join('');
            document.getElementById('sendRequestModal').classList.add('show');
        }

        function sendFriendRequest() {
            const targetId = document.getElementById('requestTargetSelect').value;
            if (selectedClientId && targetId) {
                runCommand('send-request', [selectedClientId, targetId]);
            }
            hideModals();
        }

        function showAddFriendModal() {
            const select = document.getElementById('friendSelect');
            // Show all clients (test and real) as potential friends
            select.innerHTML = clients
                .filter(c => c.clientId !== selectedClientId)
                .map(c => '<option value="' + c.clientId + '">' + (c.isTest ? '[T] ' : '[R] ') + escapeHtml(c.displayName) + '</option>')
                .join('');
            document.getElementById('addFriendModal').classList.add('show');
        }

        function showPositionModal() {
            document.getElementById('positionModal').classList.add('show');
        }

        function hideModals() {
            document.querySelectorAll('.modal-overlay').forEach(m => m.classList.remove('show'));
        }

        function createClient() {
            const name = document.getElementById('newClientName').value.trim();
            runCommand('create-client', name ? [name] : []);
            hideModals();
            document.getElementById('newClientName').value = '';
        }

        function createPartyForSelected() {
            if (selectedClientId) {
                runCommand('create-party', [selectedClientId]);
            }
        }

        function joinParty() {
            const code = document.getElementById('partyCodeInput').value.trim().toUpperCase();
            if (selectedClientId && code) {
                runCommand('join-party', [selectedClientId, code]);
            }
            hideModals();
            document.getElementById('partyCodeInput').value = '';
        }

        function leavePartyForSelected() {
            if (selectedClientId) {
                runCommand('leave-party', [selectedClientId]);
            }
        }

        function addFriend() {
            const friendId = document.getElementById('friendSelect').value;
            if (selectedClientId && friendId) {
                runCommand('add-friend', [selectedClientId, friendId]);
            }
            hideModals();
        }

        function sendPosition() {
            const map = document.getElementById('mapSelect').value;
            const x = document.getElementById('posX').value || '0';
            const y = document.getElementById('posY').value || '0';
            const z = document.getElementById('posZ').value || '0';
            if (selectedClientId) {
                runCommand('send-position', [selectedClientId, map, x, y, z]);
            }
            hideModals();
        }

        function removeSelected() {
            if (selectedClientId) {
                runCommand('remove-client', [selectedClientId]);
                selectedClientId = null;
            }
        }

        function cleanup() {
            runCommand('cleanup', []);
            selectedClientId = null;
        }

        function sendCommand() {
            const input = document.getElementById('commandInput');
            const cmd = input.value.trim();
            if (!cmd) return;

            const parts = cmd.split(/\s+/);
            runCommand(parts[0], parts.slice(1));
            input.value = '';
            hideAutocomplete();
        }

        function runCommand(command, args) {
            const showCommands = document.getElementById('showCommands').checked;
            if (showCommands) {
                addOutput('> ' + command + ' ' + args.join(' '), 'command');
            }

            fetch('/api/command', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({command: command, args: args})
            })
            .then(r => r.json())
            .then(data => {
                if (showCommands) {
                    if (data.success) {
                        addOutput(data.message, 'success');
                    } else {
                        addOutput('Error: ' + data.error, 'error');
                    }
                }
            })
            .catch(err => {
                if (showCommands) {
                    addOutput('Error: ' + err.message, 'error');
                }
            });
        }

        function addOutput(text, type) {
            const output = document.getElementById('output');
            const time = new Date().toLocaleTimeString();
            const div = document.createElement('div');
            div.className = 'output-line ' + type;
            div.setAttribute('data-type', 'command');
            div.innerHTML = '<span class="timestamp">[' + time + ']</span>' + escapeHtml(text);
            output.appendChild(div);
            output.scrollTop = output.scrollHeight;
            applyFilters();
        }

        function addLogLine(log) {
            const output = document.getElementById('output');
            const div = document.createElement('div');
            div.className = 'output-line log';
            div.setAttribute('data-type', 'log');
            div.innerHTML = '<span class="timestamp">[' + log.time + ']</span><span class="level">' + log.level + '</span> ' + escapeHtml(log.message);
            output.appendChild(div);
            output.scrollTop = output.scrollHeight;
            applyFilters();
        }

        function applyFilters() {
            const showCommands = document.getElementById('showCommands').checked;
            const showLogs = document.getElementById('showLogs').checked;
            const lines = document.querySelectorAll('.output-line');

            lines.forEach(line => {
                const type = line.getAttribute('data-type');
                if (type === 'command') {
                    line.classList.toggle('hidden', !showCommands);
                } else if (type === 'log') {
                    line.classList.toggle('hidden', !showLogs);
                }
            });
        }

        function handleInputKeydown(event) {
            const ac = document.getElementById('autocomplete');

            if (event.key === 'Tab') {
                event.preventDefault();
                if (autocompleteSuggestions.length > 0) {
                    if (autocompleteIndex < 0) autocompleteIndex = 0;
                    applyAutocomplete(autocompleteSuggestions[autocompleteIndex]);
                } else {
                    fetchAutocomplete();
                }
            } else if (event.key === 'ArrowDown' && ac.classList.contains('show')) {
                event.preventDefault();
                autocompleteIndex = Math.min(autocompleteIndex + 1, autocompleteSuggestions.length - 1);
                updateAutocompleteSelection();
            } else if (event.key === 'ArrowUp' && ac.classList.contains('show')) {
                event.preventDefault();
                autocompleteIndex = Math.max(autocompleteIndex - 1, 0);
                updateAutocompleteSelection();
            } else if (event.key === 'Enter') {
                if (ac.classList.contains('show') && autocompleteIndex >= 0) {
                    event.preventDefault();
                    applyAutocomplete(autocompleteSuggestions[autocompleteIndex]);
                } else {
                    sendCommand();
                }
            } else if (event.key === 'Escape') {
                hideAutocomplete();
            }
        }

        function handleInputChange() {
            hideAutocomplete();
        }

        function fetchAutocomplete() {
            const input = document.getElementById('commandInput');
            fetch('/api/autocomplete?q=' + encodeURIComponent(input.value))
                .then(r => r.json())
                .then(suggestions => {
                    autocompleteSuggestions = suggestions || [];
                    if (autocompleteSuggestions.length > 0) {
                        showAutocomplete();
                    }
                });
        }

        function showAutocomplete() {
            const ac = document.getElementById('autocomplete');
            ac.innerHTML = autocompleteSuggestions.map((s, i) =>
                '<div class="autocomplete-item" onclick="applyAutocomplete(\'' + escapeHtml(s) + '\')">' + escapeHtml(s) + '</div>'
            ).join('');
            ac.classList.add('show');
            autocompleteIndex = -1;
        }

        function hideAutocomplete() {
            document.getElementById('autocomplete').classList.remove('show');
            autocompleteIndex = -1;
            autocompleteSuggestions = [];
        }

        function updateAutocompleteSelection() {
            const items = document.querySelectorAll('.autocomplete-item');
            items.forEach((item, i) => {
                item.classList.toggle('selected', i === autocompleteIndex);
            });
        }

        function applyAutocomplete(value) {
            const input = document.getElementById('commandInput');
            const parts = input.value.split(/\s+/);

            // Extract just the ID if it's in format "id (name)"
            const match = value.match(/^([^\s(]+)/);
            const cleanValue = match ? match[1] : value;

            if (parts.length <= 1) {
                input.value = cleanValue + ' ';
            } else {
                parts[parts.length - 1] = cleanValue;
                input.value = parts.join(' ') + ' ';
            }

            hideAutocomplete();
            input.focus();
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Toggle handlers - apply filters in real-time
        document.getElementById('showCommands').addEventListener('change', applyFilters);
        document.getElementById('showLogs').addEventListener('change', applyFilters);

        // Close modals on overlay click
        document.querySelectorAll('.modal-overlay').forEach(overlay => {
            overlay.addEventListener('click', function(e) {
                if (e.target === this) hideModals();
            });
        });

        // Enter key in modals
        document.getElementById('newClientName').addEventListener('keydown', e => { if (e.key === 'Enter') createClient(); });
        document.getElementById('partyCodeInput').addEventListener('keydown', e => { if (e.key === 'Enter') joinParty(); });

        connect();
    </script>
</body>
</html>
`
