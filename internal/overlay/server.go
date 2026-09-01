package overlay

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/updater"

	"github.com/gorilla/websocket"
)

// PartyPosition holds the last known state for a party member.
type PartyPosition struct {
	RemoteID    string                 `json:"remoteId"`
	DisplayName string                 `json:"displayName"`
	Position    map[string]interface{} `json:"position"`
	Rotation    float64                `json:"rotation"`
	Map         string                 `json:"map"`
	LastUpdate  time.Time              `json:"lastUpdate"`
}

// Server represents the local overlay HTTP + WebSocket server.
// It serves:
//   - GET /nexus/map   → built-in Leaflet map viewer page
//   - GET /nexus/state → current state as JSON (for map page on load)
//   - WS  /ws          → WebSocket for map viewer page
//   - WS  /            → WebSocket (backward-compat for Tampermonkey script)
//   - *   /*           → reverse proxy to tarkov.dev (for iframe map window)
type Server struct {
	port     int
	logger   logger.Logger
	upgrader websocket.Upgrader

	// WebSocket clients
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	writeMu   sync.Mutex // gorilla/websocket allows only one writer at a time per connection

	// Shared state (for sending to new WS clients and the /api/state endpoint)
	currentMap     string
	selfRemoteID   string
	partyPositions map[string]*PartyPosition
	stateMu        sync.RWMutex

	server *http.Server

	// Callback fired when the map page reports its session ID back.
	onRemoteIDCaptured func(remoteID string)
}

// NewServer creates a new overlay server.
func NewServer(port int, log logger.Logger) *Server {
	return &Server{
		port:   port,
		logger: log,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients:        make(map[*websocket.Conn]bool),
		partyPositions: make(map[string]*PartyPosition),
	}
}

// Start starts the HTTP + WebSocket server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)               // WS for map viewer
	mux.HandleFunc("/nexus/map", s.handleMapPage)          // built-in Leaflet map viewer
	mux.HandleFunc("/nexus/state", s.handleState)          // JSON state endpoint
	mux.HandleFunc("/nexus/capture-id", s.handleCaptureID) // receives session ID from injected script
	mux.HandleFunc("/", s.handleRoot)                      // catch-all: WS upgrade or proxy to tarkov.dev

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}

	s.logger.Info(fmt.Sprintf("Overlay server starting on port %d (map page at /map)", s.port))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(fmt.Sprintf("Overlay server error: %v", err))
		}
	}()

	return nil
}

// Stop stops the server and closes all client connections.
func (s *Server) Stop() error {
	s.logger.Info("Stopping overlay server…")

	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
		delete(s.clients, conn)
	}
	s.clientsMu.Unlock()

	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// SetCurrentMap updates the tracked current map and broadcasts the change.
func (s *Server) SetCurrentMap(mapName string) {
	s.stateMu.Lock()
	s.currentMap = mapName
	s.stateMu.Unlock()

	s.broadcastRaw(map[string]interface{}{
		"type": "mapChange",
		"map":  mapName,
	})
}

// SetSelfRemoteID stores the local player's tarkov.dev Remote ID so the map
// page can highlight the player's own marker differently.
func (s *Server) SetSelfRemoteID(remoteID string) {
	s.stateMu.Lock()
	s.selfRemoteID = remoteID
	s.stateMu.Unlock()
}

// OnRemoteIDCaptured registers a callback that fires when the map page
// reports its tarkov.dev session ID back to the server.
func (s *Server) OnRemoteIDCaptured(fn func(remoteID string)) {
	s.onRemoteIDCaptured = fn
}

// Broadcast sends a message to all connected clients and updates internal state
// when the message is a partyPosition or partyMemberLeft event.
func (s *Server) Broadcast(message interface{}) {
	// Update internal state so new clients get current data
	s.updateStateFromMessage(message)
	s.broadcastRaw(message)
}

// broadcastRaw marshals and sends message to every connected client.
func (s *Server) broadcastRaw(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal overlay message: %v", err))
		return
	}

	s.clientsMu.RLock()
	conns := make([]*websocket.Conn, 0, len(s.clients))
	for c := range s.clients {
		conns = append(conns, c)
	}
	s.clientsMu.RUnlock()

	for _, c := range conns {
		if err := s.writeMessage(c, data); err != nil {
			s.removeClient(c)
		}
	}
}

// writeMessage serializes WebSocket writes. gorilla/websocket panics with
// "concurrent write to websocket connection" if WriteMessage is called
// concurrently on the same connection — including from broadcast goroutines
// overlapping with sendFullState or a second Broadcast.
func (s *Server) writeMessage(conn *websocket.Conn, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, data)
}

// updateStateFromMessage inspects message and maintains partyPositions / currentMap.
func (s *Server) updateStateFromMessage(message interface{}) {
	msgMap, ok := message.(map[string]interface{})
	if !ok {
		return
	}

	msgType, _ := msgMap["type"].(string)
	switch msgType {
	case "partyPosition":
		dataRaw, ok := msgMap["data"].(map[string]interface{})
		if !ok {
			return
		}
		remoteID, _ := dataRaw["remoteId"].(string)
		if remoteID == "" {
			return
		}
		pp := &PartyPosition{
			RemoteID:   remoteID,
			LastUpdate: time.Now(),
		}
		if v, ok := dataRaw["displayName"].(string); ok {
			pp.DisplayName = v
		}
		if v, ok := dataRaw["position"].(map[string]interface{}); ok {
			pp.Position = v
		}
		if v, ok := dataRaw["rotation"].(float64); ok {
			pp.Rotation = v
		}
		if v, ok := dataRaw["map"].(string); ok {
			pp.Map = v
		}
		s.stateMu.Lock()
		s.partyPositions[remoteID] = pp
		s.stateMu.Unlock()

	case "partyMemberLeft":
		dataRaw, _ := msgMap["data"].(map[string]interface{})
		if dataRaw != nil {
			if remoteID, ok := dataRaw["remoteId"].(string); ok && remoteID != "" {
				s.stateMu.Lock()
				delete(s.partyPositions, remoteID)
				s.stateMu.Unlock()
			}
		}

	case "mapChange":
		if m, ok := msgMap["map"].(string); ok && m != "" {
			s.stateMu.Lock()
			s.currentMap = m
			s.stateMu.Unlock()
		}
	}
}

// removeClient safely closes and removes a client connection.
func (s *Server) removeClient(conn *websocket.Conn) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if _, ok := s.clients[conn]; ok {
		conn.Close()
		delete(s.clients, conn)
	}
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

func (s *Server) handleMapPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, mapPageHTML)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	state := s.snapshotState()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(state)
}

// handleCaptureID receives the tarkov.dev session ID from the injected script.
// POST /nexus/capture-id  body: {"sessionId":"ABCD"}
func (s *Server) handleCaptureID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.logger.Info(fmt.Sprintf("Captured Remote ID from map: %s", body.SessionID))

	if s.onRemoteIDCaptured != nil {
		s.onRemoteIDCaptured(body.SessionID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to upgrade overlay connection: %v", err))
		return
	}

	s.clientsMu.Lock()
	s.clients[conn] = true
	count := len(s.clients)
	s.clientsMu.Unlock()

	s.logger.Info(fmt.Sprintf("Overlay client connected (total: %d)", count))

	// Send current full state to this new client
	s.sendFullState(conn)

	go func() {
		defer func() {
			s.removeClient(conn)
			s.clientsMu.RLock()
			remaining := len(s.clients)
			s.clientsMu.RUnlock()
			s.logger.Info(fmt.Sprintf("Overlay client disconnected (remaining: %d)", remaining))
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

// snapshotState copies overlay state so JSON encoding cannot race with
// updateStateFromMessage replacing partyPositions entries.
func (s *Server) snapshotState() map[string]interface{} {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	positions := make(map[string]*PartyPosition, len(s.partyPositions))
	for id, pp := range s.partyPositions {
		positions[id] = pp
	}
	return map[string]interface{}{
		"currentMap":     s.currentMap,
		"selfRemoteId":   s.selfRemoteID,
		"partyPositions": positions,
	}
}

// sendFullState sends a fullState message to a single newly-connected client.
func (s *Server) sendFullState(conn *websocket.Conn) {
	state := s.snapshotState()
	state["type"] = "fullState"

	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := s.writeMessage(conn, data); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to send overlay full state: %v", err))
		s.removeClient(conn)
	}
}

// ── Root handler ──────────────────────────────────────────────────────────
//
// Catch-all for "/": dispatches WebSocket upgrades to the WS handler,
// everything else to the tarkov.dev reverse proxy.  This means the ENTIRE
// localhost:44444 origin acts as a transparent proxy for tarkov.dev, so
// in-page navigation inside the iframe never leaves localhost and never
// hits tarkov.dev's X-Frame-Options block.

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// WebSocket upgrade → overlay WS handler (backward-compat for Tampermonkey)
	if r.Header.Get("Upgrade") == "websocket" {
		s.handleWebSocket(w, r)
		return
	}
	// Everything else → proxy to tarkov.dev
	s.handleProxy(w, r)
}

// frameAncestorsPolicy restricts who may embed the proxied tarkov.dev pages.
// Only the Wails map window and this server's own viewer page qualify; every
// other origin, including any website the user happens to have open, is refused.
const frameAncestorsPolicy = "frame-ancestors 'self' http://wails.localhost https://wails.localhost"

// acceptsGzip reports whether an Accept-Encoding header actually permits gzip.
//
// A substring test is not enough: "gzip;q=0" *names* gzip in order to refuse it,
// and a bare "*" accepts it without naming it. Responses are forwarded verbatim,
// so asking upstream for an encoding the caller rejected would hand it bytes it
// cannot decode.
func acceptsGzip(header string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}

	wildcard := false
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		coding := strings.ToLower(strings.TrimSpace(fields[0]))
		if coding != "gzip" && coding != "*" {
			continue
		}

		// Any q-value of zero is an explicit refusal of that coding.
		acceptable := true
		for _, param := range fields[1:] {
			param = strings.ToLower(strings.TrimSpace(param))
			if !strings.HasPrefix(param, "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil && q <= 0 {
				acceptable = false
			}
		}

		if coding == "gzip" {
			// An explicit gzip entry is authoritative, accept or refuse.
			return acceptable
		}
		wildcard = wildcard || acceptable
	}

	return wildcard
}

// rewriteLocalReferer maps a referer pointing at the local proxy back to the
// tarkov.dev URL it actually represents, so forwarded requests carry a truthful
// referer instead of none at all.
func rewriteLocalReferer(ref string) string {
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "https://tarkov.dev/"
	}
	// Exact hostname match, not a prefix: "localhost.evil.example" starts with
	// "localhost" but is a different origin entirely, and rewriting it would
	// forge a tarkov.dev referer on behalf of an attacker-controlled page.
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
	default:
		// Not ours to rewrite.
		return ref
	}
	u.Scheme = "https"
	u.Host = "tarkov.dev"
	return u.String()
}

// ── Tarkov.dev reverse proxy ───────────────────────────────────────────────
//
// Proxies every non-WS, non-/nexus/* request to https://tarkov.dev.
// For HTML responses the proxy:
//   - Strips X-Frame-Options and Content-Security-Policy so the page can be
//     loaded inside an iframe served from localhost.
//   - Injects the party-markers script so markers appear without Tampermonkey.
//
// Because the ENTIRE origin proxies tarkov.dev, no <base href> rewrite is
// needed — all relative and absolute paths resolve against localhost:44444
// which forwards them to tarkov.dev transparently.

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse("https://tarkov.dev")
	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = "https"
		req.URL.Host = "tarkov.dev"
		req.Host = "tarkov.dev"
		// Path is forwarded as-is (no prefix to strip)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}

		// Identify this traffic. tarkov.dev blocked an earlier version of this
		// application partly because its requests could not be attributed to
		// anyone (see docs/tarkov-dev-remediation.md). Everything we fetch on
		// the user's behalf now says who is asking and on which version.
		req.Header.Set("X-Tarkov-Nexus-Version", updater.Version)

		// Prefer gzip so assets stay compressed on the wire — that is
		// tarkov.dev's bandwidth, not ours — and gzip is the one encoding the
		// standard library can decode for the HTML we rewrite. But only when
		// the caller can actually accept it: responses are forwarded verbatim,
		// so asking upstream for an encoding the caller did not offer would
		// hand it bytes it cannot decode.
		if acceptsGzip(req.Header.Get("Accept-Encoding")) {
			req.Header.Set("Accept-Encoding", "gzip")
		} else {
			req.Header.Set("Accept-Encoding", "identity")
		}

		// These are subresource loads for a tarkov.dev page being viewed in
		// the app. Rewriting the Referer to match is more honest than deleting
		// it: a request carrying neither Origin nor Referer looks like a
		// scraper, which is the opposite of what we want in their logs.
		if ref := req.Header.Get("Referer"); ref != "" {
			req.Header.Set("Referer", rewriteLocalReferer(ref))
		}
		req.Header.Del("Origin")
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		// Remove X-Frame-Options, and nothing else.
		//
		// Verified against tarkov.dev: they send `X-Frame-Options: DENY` and a
		// Content-Security-Policy that contains **no** frame-ancestors
		// directive, so dropping this single header is all that embedding
		// requires. Their CSP and X-Content-Type-Options are left intact —
		// deleting those downgraded the user's security for no functional
		// gain. The CSP allows 'unsafe-inline' scripts, so the party-marker
		// script below still runs under it, and it declares no connect-src, so
		// the local WebSocket connection is unaffected.
		//
		// Their own CORS policy is likewise left alone: overriding it with a
		// wildcard made their responses readable by any origin, which is not
		// ours to decide.
		resp.Header.Del("X-Frame-Options")

		// Removing X-Frame-Options without putting anything back would let *any*
		// page the user visits frame this proxy — clickjacking tarkov.dev's UI in
		// the user's session, and letting a third-party page drive traffic to
		// tarkov.dev through this machine, which is the attribution problem this
		// proxy exists to avoid. Binding to 127.0.0.1 does not help: it limits
		// who can reach the port, not who can embed it.
		//
		// This is *added*, not merged into their policy. Browsers enforce every
		// Content-Security-Policy header independently and take the intersection,
		// so a second header can only further restrict — tarkov.dev's own policy
		// passes through byte-identical.
		//
		// The allowed parent is the Wails asset origin: wails/v2@v2.15.0 sets
		// `const startURL = "http://wails.localhost/"` for the Windows WebView2
		// frontend, which is what hosts the map window's iframe (unchanged from
		// v2.11.0, re-checked at the v2.15.0 upgrade). 'self' covers the built-in
		// viewer at /nexus/map framing it from this same origin.
		resp.Header.Add("Content-Security-Policy", frameAncestorsPolicy)

		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			return nil
		}

		// Decompress if tarkov.dev still sent gzip despite our Accept-Encoding
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil
			}
			defer gr.Close()
			reader = gr
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Transfer-Encoding")
		}

		raw, err := io.ReadAll(reader)
		if err != nil {
			return nil
		}
		resp.Body.Close()

		html := string(raw)

		// No <base href> needed — the whole origin proxies tarkov.dev.

		// Nothing is injected before React boots. An earlier version
		// monkey-patched localStorage.getItem so that tarkov.dev's own
		// "savedMapSettings" read returned filter defaults we had chosen. That
		// silently overrode the user's settings on someone else's site; map
		// filters are theirs to set.

		// Inject party-markers script before </body>
		script := "<script>" + proxyInjectScript + "</script>"
		if idx := strings.LastIndex(html, "</body>"); idx != -1 {
			html = html[:idx] + script + html[idx:]
		} else {
			html += script
		}

		b := []byte(html)
		resp.Body = io.NopCloser(bytes.NewReader(b))
		resp.ContentLength = int64(len(b))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error(fmt.Sprintf("Tarkov proxy error: %v", err))
		http.Error(w, "proxy error", http.StatusBadGateway)
	}

	// Everything inside the map window is same-origin against
	// localhost:44444, so only an actual preflight needs a CORS answer.
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	proxy.ServeHTTP(w, r)
}
