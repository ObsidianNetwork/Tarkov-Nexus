# Tarkov Nexus Makefile
#
# Layout: the desktop app is built with Wails from ui-wails/; party-server/ is a
# separate Go module with its own go.mod. Setup and the documented Wails
# startup path are in README.md > Development.
#
# Note: recipes use POSIX commands (rm, date, grep). On Windows, run make from
# Git Bash so an sh-compatible shell is available.
#
# Important: `go test` / `go vet` / `go mod tidy` on the root module load
# ui-wails/main.go, which embeds frontend/dist via `//go:embed all:frontend/dist`.
# The frontend must be built first, so those targets depend on `frontend`.

VERSION?=$(shell grep -E '(const|var) Version = ' internal/updater/version.go | sed 's/.*"\(.*\)".*/\1/')
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

FRONTEND_DIR=ui-wails/frontend
WAILS_DIR=ui-wails
PARTY_DIR=party-server

.PHONY: all frontend build dev clean test vet fmt lint deps tidy \
        party-build party-test party-docker version help

# Default target: build the embedded frontend, then the desktop app.
all: build

# Install frontend dependencies and build the assets embedded by the Go binary.
frontend:
	@echo "📦 Building frontend..."
	@cd ${FRONTEND_DIR} && npm ci && npm run build

# Build the Wails desktop application (requires the Wails CLI).
build: frontend
	@echo "🔨 Building desktop app..."
	@cd ${WAILS_DIR} && wails build -platform windows/amd64

# Run the desktop app in development mode with hot reload.
dev:
	@echo "🔄 Starting wails dev..."
	@cd ${WAILS_DIR} && wails dev

# Remove build output from both the Go build cache and the frontend.
clean:
	@echo "🧹 Cleaning..."
	@rm -rf ${WAILS_DIR}/build/bin ${FRONTEND_DIR}/dist
	@go clean

# Run root-module tests (desktop app + internal packages).
test: frontend
	@echo "🧪 Running root module tests..."
	@go test ./... -v

vet: frontend
	@echo "🔍 Running go vet..."
	@go vet ./...

fmt:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@cd ${PARTY_DIR} && go fmt ./...

# Requires golangci-lint on PATH.
lint:
	@echo "🔍 Running linter..."
	@golangci-lint run

deps:
	@echo "📦 Downloading module dependencies..."
	@go mod download
	@cd ${PARTY_DIR} && go mod download

tidy: frontend
	@echo "🧹 Tidying dependencies..."
	@go mod tidy
	@cd ${PARTY_DIR} && go mod tidy

# --- party-server (separate Go module) ---

party-build:
	@echo "🔨 Building party server..."
	@cd ${PARTY_DIR} && go build -o bin/party-server ./cmd/server

party-test:
	@echo "🧪 Running party server tests..."
	@cd ${PARTY_DIR} && go test ./... -v

# Requires Docker; uses party-server/Dockerfile.
party-docker:
	@echo "🐳 Building party server image..."
	@cd ${PARTY_DIR} && docker build -t tarkov-nexus-party-server .

version:
	@echo "Version:    ${VERSION}"
	@echo "Build Time: ${BUILD_TIME}"
	@echo "Git Commit: ${GIT_COMMIT}"

help:
	@echo "Tarkov Nexus - Available targets:"
	@echo ""
	@echo "  Desktop app (root Go module + ui-wails)"
	@echo "    frontend      - Install frontend deps and build embedded assets"
	@echo "    build         - Build the Wails desktop app (implies frontend)"
	@echo "    dev           - Run 'wails dev' with hot reload"
	@echo "    test          - Run root module tests (implies frontend)"
	@echo "    vet           - Run go vet (implies frontend)"
	@echo "    clean         - Remove frontend dist and build output"
	@echo ""
	@echo "  Party server (separate Go module in party-server/)"
	@echo "    party-build   - Build the party server binary"
	@echo "    party-test    - Run party server tests"
	@echo "    party-docker  - Build the party server Docker image"
	@echo ""
	@echo "  Shared"
	@echo "    deps          - go mod download for both modules"
	@echo "    tidy          - go mod tidy for both modules (implies frontend)"
	@echo "    fmt           - Format Go code in both modules"
	@echo "    lint          - Run golangci-lint"
	@echo "    version       - Show version information"
	@echo "    help          - Show this help message"
	@echo ""
	@echo "Example usage:"
	@echo "  make dev        # Run the desktop app with hot reload"
	@echo "  make build      # Build the production executable"
	@echo "  make test       # Build the frontend, then run Go tests"
