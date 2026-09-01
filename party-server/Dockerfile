# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o party-server ./cmd/server

# Build the CLI tool
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-s -w' -o party-cli ./cmd/cli

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Unraid labels (must be in image for Unraid to detect)
LABEL net.unraid.docker.webui="http://[IP]:[PORT:8766]/"
LABEL net.unraid.docker.icon="https://raw.githubusercontent.com/Jordanmuss99/Tarkov-Nexus/main/Branding/Obi_logo.png"
LABEL maintainer="Jordanmuss99"
LABEL description="Tarkov Party Server - Real-time position sync for Escape from Tarkov"

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates tzdata

# Create data directory
RUN mkdir -p /app/data

# Copy binaries from builder
COPY --from=builder /app/party-server .
COPY --from=builder /app/party-cli .

# Expose ports (8765 = WebSocket API, 8766 = Web UI Console)
EXPOSE 8765 8766

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8765/health || exit 1

# Volume for persistent data
VOLUME ["/app/data"]

# Run the server with dev tools enabled (console + admin API)
ENTRYPOINT ["./party-server"]
CMD ["-port", "8765", "-data", "/app/data", "-dev"]
