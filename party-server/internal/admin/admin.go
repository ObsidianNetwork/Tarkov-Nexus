package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tarkov-screenshot-analyzer/party-server/internal/hub"
)

// AdminAPI provides HTTP endpoints for testing and debugging
type AdminAPI struct {
	hub *hub.Hub
}

// NewAdminAPI creates a new admin API handler
func NewAdminAPI(h *hub.Hub) *AdminAPI {
	return &AdminAPI{hub: h}
}

// RegisterRoutes registers admin routes on the given mux
func (a *AdminAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/clients", a.handleClients)
	mux.HandleFunc("/admin/parties", a.handleParties)
	mux.HandleFunc("/admin/test-client", a.handleTestClient)
	mux.HandleFunc("/admin/test-party", a.handleTestParty)
	mux.HandleFunc("/admin/test-friends", a.handleTestFriends)
	mux.HandleFunc("/admin/test-position", a.handleTestPosition)
	mux.HandleFunc("/admin/cleanup", a.handleCleanup)

	log.Println("[Admin] Admin API endpoints registered")
}

// handleClients returns list of all connected clients
func (a *AdminAPI) handleClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clients := a.hub.GetAllClients()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(clients),
		"clients": clients,
	})
}

// handleParties returns list of all active parties
func (a *AdminAPI) handleParties(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parties := a.hub.GetAllParties()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(parties),
		"parties": parties,
	})
}

// TestClientRequest represents a request to create a test client
type TestClientRequest struct {
	DisplayName string `json:"displayName"`
	ClientID    string `json:"clientId,omitempty"`
	RemoteID    string `json:"remoteId,omitempty"`
}

// handleTestClient creates or removes a test client
func (a *AdminAPI) handleTestClient(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		var req TestClientRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.DisplayName == "" {
			req.DisplayName = fmt.Sprintf("TestUser_%d", time.Now().UnixNano()%10000)
		}

		client, err := a.hub.CreateTestClient(req.DisplayName, req.ClientID, req.RemoteID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[Admin] Created test client: %s (%s)", client.DisplayName, client.ID[:8])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"clientId":    client.ID,
			"displayName": client.DisplayName,
			"remoteId":    client.RemoteID,
		})

	case http.MethodDelete:
		clientID := r.URL.Query().Get("clientId")
		if clientID == "" {
			http.Error(w, "clientId required", http.StatusBadRequest)
			return
		}

		if err := a.hub.RemoveTestClient(clientID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		log.Printf("[Admin] Removed test client: %s", clientID[:8])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"clientId": clientID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// TestPartyRequest represents a request to create a test party
type TestPartyRequest struct {
	HostClientID string   `json:"hostClientId"`
	MemberIDs    []string `json:"memberIds,omitempty"`
}

// handleTestParty creates a test party
func (a *AdminAPI) handleTestParty(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		var req TestPartyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.HostClientID == "" {
			http.Error(w, "hostClientId required", http.StatusBadRequest)
			return
		}

		partyCode, err := a.hub.CreateTestParty(req.HostClientID, req.MemberIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("[Admin] Created test party: %s", partyCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"partyCode": partyCode,
		})

	case http.MethodDelete:
		partyCode := strings.ToUpper(r.URL.Query().Get("partyCode"))
		if partyCode == "" {
			http.Error(w, "partyCode required", http.StatusBadRequest)
			return
		}

		if err := a.hub.DeleteParty(partyCode); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		log.Printf("[Admin] Deleted party: %s", partyCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"partyCode": partyCode,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// TestFriendsRequest represents a request to create friend relationship
type TestFriendsRequest struct {
	ClientID1 string `json:"clientId1"`
	ClientID2 string `json:"clientId2"`
}

// handleTestFriends creates a bidirectional friend relationship
func (a *AdminAPI) handleTestFriends(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		var req TestFriendsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.ClientID1 == "" || req.ClientID2 == "" {
			http.Error(w, "clientId1 and clientId2 required", http.StatusBadRequest)
			return
		}

		if err := a.hub.CreateTestFriendship(req.ClientID1, req.ClientID2); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("[Admin] Created test friendship: %s <-> %s", req.ClientID1[:8], req.ClientID2[:8])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"clientId1": req.ClientID1,
			"clientId2": req.ClientID2,
		})

	case http.MethodDelete:
		clientID1 := r.URL.Query().Get("clientId1")
		clientID2 := r.URL.Query().Get("clientId2")
		if clientID1 == "" || clientID2 == "" {
			http.Error(w, "clientId1 and clientId2 required", http.StatusBadRequest)
			return
		}

		if err := a.hub.RemoveTestFriendship(clientID1, clientID2); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("[Admin] Removed friendship: %s <-> %s", clientID1[:8], clientID2[:8])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"clientId1": clientID1,
			"clientId2": clientID2,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// TestPositionRequest represents a request to send a test position
type TestPositionRequest struct {
	ClientID string  `json:"clientId"`
	Map      string  `json:"map"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Rotation float64 `json:"rotation"`
}

// handleTestPosition sends a test position update for a client
func (a *AdminAPI) handleTestPosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req TestPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ClientID == "" || req.Map == "" {
		http.Error(w, "clientId and map required", http.StatusBadRequest)
		return
	}

	if err := a.hub.SendTestPosition(req.ClientID, req.Map, req.X, req.Y, req.Z, req.Rotation); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[Admin] Sent test position for %s on %s: (%.1f, %.1f, %.1f)", req.ClientID[:8], req.Map, req.X, req.Y, req.Z)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// handleCleanup removes all test clients and their data
func (a *AdminAPI) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	count := a.hub.CleanupTestClients()
	log.Printf("[Admin] Cleaned up %d test clients", count)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"clientsRemoved": count,
	})
}
