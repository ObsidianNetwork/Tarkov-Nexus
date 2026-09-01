# Tarkov Party Server

A centralized WebSocket server for the Tarkov Map Sync party system. Provides party management, position broadcasting, and friend list functionality.

## Features

- **Party Management**: Create and join parties with simple invite codes
- **Position Broadcasting**: Real-time position sharing between party members
- **Friends System**: Add friends, see online status, send party invites
- **Persistent Storage**: SQLite database for friend relationships
- **Docker Ready**: Easy deployment on Unraid or any Docker host

## Quick Start

### Using Docker Compose

```bash
cd party-server
docker-compose up -d
```

### Using Docker directly

```bash
docker build -t tarkov-party-server .
docker run -d \
  --name tarkov-party-server \
  -p 8765:8765 \
  -v party-data:/app/data \
  tarkov-party-server
```

### Running locally (development)

```bash
cd party-server
go run ./cmd/server -port 8765 -data ./data
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| TZ | UTC | Timezone for logs |

### Command Line Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| -port | 8765 | Server port |
| -data | ./data | Data directory for SQLite |
| -console | false | Enable interactive dev console |
| -admin | false | Enable admin API endpoints |
| -dev | false | Enable dev mode (console + admin)

## Unraid Deployment

1. Copy the Dockerfile and docker-compose.yml to your Unraid server
2. Build the image:
   ```bash
   docker build -t tarkov-party-server .
   ```
3. Or use Docker Compose:
   ```bash
   docker-compose up -d
   ```
4. Configure port forwarding if needed for external access

### Unraid Template (manual)

- **Repository**: tarkov-party-server
- **Network**: bridge
- **Port**: 8765:8765
- **Path**: /mnt/user/appdata/tarkov-party-server:/app/data

## API Endpoints

### WebSocket: `/ws`

Main WebSocket endpoint for client connections.

### HTTP: `/health`

Health check endpoint. Returns:
```json
{"status": "healthy", "time": "2024-01-15T12:00:00Z"}
```

### HTTP: `/stats`

Server statistics. Returns:
```json
{"clients": 10, "parties": 3, "uptime": "2024-01-15T12:00:00Z"}
```

## WebSocket Protocol

### Client → Server Messages

#### Register
```json
{"type": "register", "clientId": "uuid", "displayName": "PlayerName"}
```

#### Create Party
```json
{"type": "create_party"}
```

#### Join Party
```json
{"type": "join_party", "partyCode": "ALPHA-1234"}
```

#### Leave Party
```json
{"type": "leave_party"}
```

#### Position Update
```json
{"type": "position", "map": "customs", "x": 123.4, "y": 56.7, "z": 8.9, "rotation": 180.0}
```

#### Friend Management
```json
{"type": "add_friend", "targetClientId": "uuid"}
{"type": "remove_friend", "targetClientId": "uuid"}
{"type": "get_friends"}
{"type": "invite_friend", "targetClientId": "uuid"}
{"type": "accept_invite", "partyCode": "ALPHA-1234"}
{"type": "decline_invite", "partyCode": "ALPHA-1234"}
```

### Server → Client Messages

#### Registration Confirmed
```json
{"type": "registered", "clientId": "uuid"}
```

#### Party Created
```json
{"type": "party_created", "partyCode": "ALPHA-1234"}
```

#### Party Joined
```json
{
  "type": "party_joined",
  "partyCode": "ALPHA-1234",
  "members": [
    {"clientId": "uuid", "displayName": "Player1", "isHost": true}
  ]
}
```

#### Position Update (from other members)
```json
{
  "type": "position_update",
  "clientId": "uuid",
  "displayName": "Player1",
  "map": "customs",
  "position": {"x": 123.4, "y": 56.7, "z": 8.9, "rotation": 180.0}
}
```

#### Friends List
```json
{
  "type": "friends_list",
  "friends": [
    {"clientId": "uuid", "displayName": "Friend1", "online": true, "inParty": false}
  ]
}
```

#### Party Invite
```json
{
  "type": "party_invite",
  "fromClientId": "uuid",
  "fromDisplayName": "Player1",
  "partyCode": "ALPHA-1234"
}
```

## Data Storage

SQLite database stored in `/app/data/party.db`:

### Tables

- **clients**: Registered clients with display names
- **friends**: Friend relationships (bidirectional)

## Security Notes

- Party codes are randomly generated NATO phonetic words + 4 digits
- No authentication required (anonymous sessions)
- Friends are bidirectional (both users become friends)
- Invite codes expire after 5 minutes
- Parties are ephemeral (deleted when empty)

## Development

### Project Structure

```
party-server/
├── cmd/server/
│   └── main.go          # Entry point
├── internal/
│   ├── admin/
│   │   └── admin.go     # Admin HTTP API
│   ├── console/
│   │   └── console.go   # Interactive dev console
│   ├── hub/
│   │   └── hub.go       # WebSocket hub & party logic
│   ├── models/
│   │   ├── models.go    # Data structures
│   │   └── messages.go  # Message types
│   └── storage/
│       └── storage.go   # SQLite storage
├── Dockerfile
├── docker-compose.yml
├── docker-compose.dev.yml  # Dev mode with console
├── go.mod
└── README.md
```

### Building

```bash
go build -o party-server ./cmd/server
```

### Testing locally

```bash
# Terminal 1: Start server
go run ./cmd/server

# Terminal 2: Test with websocat
websocat ws://localhost:8765/ws
> {"type":"register","clientId":"test-123","displayName":"TestUser"}
```

## Dev Tools

The party server includes built-in development and debugging tools that let you test the party/friend system without needing multiple computers or real clients.

### Interactive Console

Start with `-console` or `-dev` flag to enable the interactive console:

```bash
# Local development
go run ./cmd/server -dev

# Docker development
docker-compose -f docker-compose.dev.yml up --build
```

#### Console Commands

```
CLIENT MANAGEMENT:
  clients                       List all connected clients
  create-client <name>          Create a test client
  remove-client <clientId>      Remove a test client
  cleanup                       Remove all test clients

PARTY MANAGEMENT:
  parties                       List all active parties
  create-party <clientId>       Create party for a client
  join-party <clientId> <code>  Add client to party
  delete-party <code>           Delete a party

FRIEND MANAGEMENT:
  friends <clientId>            List friends for a client
  add-friend <id1> <id2>        Make two clients friends
  remove-friend <id1> <id2>     Remove friendship

POSITION TESTING:
  pos <clientId> <map> <x> <y> <z> [rotation]
                                Send test position update

UTILITIES:
  status                        Show server status
  help                          Show available commands
  clear                         Clear the console
```

#### Example Session

```bash
> create-client Alice
Created test client:
  ID:   test-1234567890-1234
  Name: Alice

> create-client Bob
Created test client:
  ID:   test-1234567890-5678
  Name: Bob

> add-friend test-1234 test-5678
Added friendship: test-1234 <-> test-5678

> create-party test-1234
Created party: ALPHA-4567

> join-party test-5678 ALPHA-4567
Client test-5678 joined party ALPHA-4567

> pos test-1234 customs 100 50 -20
Sent position for test-1234: customs (100.0, 50.0, -20.0)

> status
┌─ Server Status ─────────────────────────────────────
│ Connected Clients: 2
│ Active Parties:    1
│ Test Clients:      2
│ Time:              2024-01-15 14:30:00
└─────────────────────────────────────────────────────
```

### Admin HTTP API

Enable with `-admin` or `-dev` flag. Provides REST endpoints for programmatic testing.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /admin/clients | List all clients |
| GET | /admin/parties | List all parties |
| POST | /admin/test-client | Create test client |
| DELETE | /admin/test-client?clientId=xxx | Remove test client |
| POST | /admin/test-party | Create test party |
| DELETE | /admin/test-party?partyCode=xxx | Delete party |
| POST | /admin/test-friends | Create friendship |
| DELETE | /admin/test-friends?clientId1=xxx&clientId2=yyy | Remove friendship |
| POST | /admin/test-position | Send position update |
| POST | /admin/cleanup | Remove all test clients |

#### Example API Calls

```bash
# Create a test client
curl -X POST http://localhost:8765/admin/test-client \
  -H "Content-Type: application/json" \
  -d '{"displayName": "TestPlayer"}'

# Create friendship between two clients
curl -X POST http://localhost:8765/admin/test-friends \
  -H "Content-Type: application/json" \
  -d '{"clientId1": "xxx", "clientId2": "yyy"}'

# Create a party
curl -X POST http://localhost:8765/admin/test-party \
  -H "Content-Type: application/json" \
  -d '{"hostClientId": "xxx", "memberIds": ["yyy", "zzz"]}'

# Send test position
curl -X POST http://localhost:8765/admin/test-position \
  -H "Content-Type: application/json" \
  -d '{"clientId": "xxx", "map": "customs", "x": 100, "y": 50, "z": -20}'

# List all clients
curl http://localhost:8765/admin/clients

# Cleanup all test clients
curl -X POST http://localhost:8765/admin/cleanup
```

### Docker Dev Mode

Use the development docker-compose file for interactive testing:

```bash
# Build and start with console enabled
docker-compose -f docker-compose.dev.yml up --build

# To attach to the console (in another terminal)
docker attach tarkov-party-server-dev

# To detach without stopping: Ctrl+P, Ctrl+Q
```

The dev docker-compose provides:
- Interactive console with stdin/tty enabled
- Admin API endpoints enabled
- Local `./data` volume mount for easy database access
- No auto-restart (for development)

## License

MIT
