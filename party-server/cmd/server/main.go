package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"tarkov-screenshot-analyzer/party-server/internal/admin"
	"tarkov-screenshot-analyzer/party-server/internal/console"
	"tarkov-screenshot-analyzer/party-server/internal/hub"
	"tarkov-screenshot-analyzer/party-server/internal/storage"
	"tarkov-screenshot-analyzer/party-server/internal/webui"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type Server struct {
	hub  *hub.Hub
	port int
}

func main() {
	port := flag.Int("port", 8765, "Server port")
	webPort := flag.Int("webui-port", 8766, "Web UI console port")
	dataDir := flag.String("data", "./data", "Data directory for storage")
	enableConsole := flag.Bool("console", false, "Enable interactive console for dev/debug")
	enableAdmin := flag.Bool("admin", false, "Enable admin API endpoints")
	enableWebUI := flag.Bool("webui", false, "Enable web UI console")
	devMode := flag.Bool("dev", false, "Enable dev mode (console + admin + webui)")
	flag.Parse()

	// Dev mode enables everything
	if *devMode {
		*enableConsole = true
		*enableAdmin = true
		*enableWebUI = true
	}

	log.Printf("Starting Tarkov Party Server on port %d", *port)
	log.Printf("Data directory: %s", *dataDir)
	if *enableConsole {
		log.Printf("Interactive console: ENABLED")
	}
	if *enableAdmin {
		log.Printf("Admin API: ENABLED")
	}
	if *enableWebUI {
		log.Printf("Web UI console: ENABLED on port %d", *webPort)
	}

	// Initialize storage
	store, err := storage.NewStorage(*dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Initialize hub
	h := hub.NewHub(store)

	// Create server
	server := &Server{
		hub:  h,
		port: *port,
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.handleWebSocket)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/stats", server.handleStats)

	// Register admin API if enabled
	if *enableAdmin {
		adminAPI := admin.NewAdminAPI(h)
		adminAPI.RegisterRoutes(mux)
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on :%d", *port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Start interactive console if enabled
	if *enableConsole {
		cons := console.NewConsole(h)
		cons.StartBackground()
	}

	// Start web UI if enabled
	if *enableWebUI {
		webUI := webui.NewWebUI(h)
		webMux := http.NewServeMux()
		webUI.RegisterRoutes(webMux)

		// Also register admin API on web UI port for convenience
		if *enableAdmin {
			adminAPI := admin.NewAdminAPI(h)
			adminAPI.RegisterRoutes(webMux)
		}

		webServer := &http.Server{
			Addr:         fmt.Sprintf(":%d", *webPort),
			Handler:      webMux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		}

		go func() {
			log.Printf("Web UI listening on :%d", *webPort)
			if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Web UI server error: %v", err)
			}
		}()
	}

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Stop the hub's background cleanup routine
	h.Close()

	log.Println("Server stopped")
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	log.Printf("New WebSocket connection from %s", r.RemoteAddr)
	s.hub.HandleConnection(conn)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := s.hub.GetStats()
	stats["uptime"] = time.Now().UTC().Format(time.RFC3339)
	json.NewEncoder(w).Encode(stats)
}
