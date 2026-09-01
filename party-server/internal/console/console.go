package console

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"tarkov-screenshot-analyzer/party-server/internal/hub"
)

// Console provides an interactive command-line interface for the party server
type Console struct {
	hub    *hub.Hub
	reader *bufio.Reader
	writer io.Writer
}

// NewConsole creates a new console instance
func NewConsole(h *hub.Hub) *Console {
	return &Console{
		hub:    h,
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
}

// Run starts the console loop
func (c *Console) Run() {
	c.printBanner()
	c.printHelp()

	for {
		fmt.Fprint(c.writer, "\n> ")
		input, err := c.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(c.writer, "\nExiting console...")
				return
			}
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		c.handleCommand(cmd, args)
	}
}

func (c *Console) printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════╗
║          TARKOV PARTY SERVER - DEV CONSOLE                ║
║                                                           ║
║  Type 'help' for available commands                       ║
╚═══════════════════════════════════════════════════════════╝
`
	fmt.Fprintln(c.writer, banner)
}

func (c *Console) printHelp() {
	help := `
Available Commands:
───────────────────────────────────────────────────────────
  help                          Show this help message
  status                        Show server status

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
  logs [on|off]                 Toggle verbose logging
  clear                         Clear the console
  exit, quit                    Exit console (server continues)
───────────────────────────────────────────────────────────
`
	fmt.Fprintln(c.writer, help)
}

func (c *Console) handleCommand(cmd string, args []string) {
	switch cmd {
	case "help", "h", "?":
		c.printHelp()

	case "status":
		c.showStatus()

	case "clients", "list-clients", "lc":
		c.listClients()

	case "create-client", "cc":
		c.createClient(args)

	case "remove-client", "rc", "delete-client", "dc":
		c.removeClient(args)

	case "cleanup":
		c.cleanup()

	case "parties", "list-parties", "lp":
		c.listParties()

	case "create-party", "cp":
		c.createParty(args)

	case "join-party", "jp":
		c.joinParty(args)

	case "delete-party", "dp":
		c.deleteParty(args)

	case "friends", "list-friends", "lf":
		c.listFriends(args)

	case "add-friend", "af":
		c.addFriend(args)

	case "remove-friend", "rf":
		c.removeFriend(args)

	case "pos", "position":
		c.sendPosition(args)

	case "clear", "cls":
		fmt.Fprint(c.writer, "\033[2J\033[H")
		c.printBanner()

	case "exit", "quit", "q":
		fmt.Fprintln(c.writer, "Console closed. Server continues running.")
		fmt.Fprintln(c.writer, "Use Ctrl+C to stop the server.")
		// Don't actually exit, just stop reading commands
		select {}

	default:
		fmt.Fprintf(c.writer, "Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

func (c *Console) showStatus() {
	stats := c.hub.GetStats()
	fmt.Fprintln(c.writer, "\n┌─ Server Status ─────────────────────────────────────")
	fmt.Fprintf(c.writer, "│ Connected Clients: %d\n", stats["clients"])
	fmt.Fprintf(c.writer, "│ Active Parties:    %d\n", stats["parties"])
	fmt.Fprintf(c.writer, "│ Test Clients:      %d\n", stats["testClients"])
	fmt.Fprintf(c.writer, "│ Time:              %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintln(c.writer, "└─────────────────────────────────────────────────────")
}

func (c *Console) listClients() {
	clients := c.hub.GetAllClients()
	if len(clients) == 0 {
		fmt.Fprintln(c.writer, "\nNo connected clients.")
		return
	}

	fmt.Fprintln(c.writer, "\n┌─ Connected Clients ─────────────────────────────────")
	for _, client := range clients {
		isTest := ""
		if client.IsTest {
			isTest = " [TEST]"
		}
		party := ""
		if client.PartyCode != "" {
			party = fmt.Sprintf(" (party: %s)", client.PartyCode)
		}
		fmt.Fprintf(c.writer, "│ %s - %s%s%s\n", client.ID[:12], client.DisplayName, party, isTest)
	}
	fmt.Fprintf(c.writer, "└─ Total: %d ───────────────────────────────────────────\n", len(clients))
}

func (c *Console) createClient(args []string) {
	name := ""
	if len(args) > 0 {
		name = strings.Join(args, " ")
	}

	client, err := c.hub.CreateTestClient(name, "", "")
	if err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Created test client:\n")
	fmt.Fprintf(c.writer, "  ID:   %s\n", client.ID)
	fmt.Fprintf(c.writer, "  Name: %s\n", client.DisplayName)
}

func (c *Console) removeClient(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(c.writer, "Usage: remove-client <clientId>")
		fmt.Fprintln(c.writer, "  Tip: You can use just the first 8+ characters of the ID")
		return
	}

	clientID := c.hub.FindClientByPrefix(args[0])
	if clientID == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	if err := c.hub.RemoveTestClient(clientID); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Removed test client: %s\n", clientID[:12])
}

func (c *Console) cleanup() {
	count := c.hub.CleanupTestClients()
	fmt.Fprintf(c.writer, "Cleaned up %d test clients\n", count)
}

func (c *Console) listParties() {
	parties := c.hub.GetAllParties()
	if len(parties) == 0 {
		fmt.Fprintln(c.writer, "\nNo active parties.")
		return
	}

	fmt.Fprintln(c.writer, "\n┌─ Active Parties ────────────────────────────────────")
	for _, party := range parties {
		fmt.Fprintf(c.writer, "│ %s (host: %s, members: %d)\n", party.Code, party.HostID[:8], party.MemberCount)
		for _, member := range party.Members {
			fmt.Fprintf(c.writer, "│   └─ %s - %s\n", member.ID[:8], member.DisplayName)
		}
	}
	fmt.Fprintf(c.writer, "└─ Total: %d ───────────────────────────────────────────\n", len(parties))
}

func (c *Console) createParty(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(c.writer, "Usage: create-party <clientId>")
		return
	}

	clientID := c.hub.FindClientByPrefix(args[0])
	if clientID == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	code, err := c.hub.CreateTestParty(clientID, nil)
	if err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Created party: %s\n", code)
}

func (c *Console) joinParty(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(c.writer, "Usage: join-party <clientId> <partyCode>")
		return
	}

	clientID := c.hub.FindClientByPrefix(args[0])
	if clientID == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	partyCode := strings.ToUpper(args[1])
	if err := c.hub.JoinTestParty(clientID, partyCode); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Client %s joined party %s\n", clientID[:8], partyCode)
}

func (c *Console) deleteParty(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(c.writer, "Usage: delete-party <partyCode>")
		return
	}

	code := strings.ToUpper(args[0])
	if err := c.hub.DeleteParty(code); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Deleted party: %s\n", code)
}

func (c *Console) listFriends(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(c.writer, "Usage: friends <clientId>")
		return
	}

	clientID := c.hub.FindClientByPrefix(args[0])
	if clientID == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	friends, err := c.hub.GetFriendsForClient(clientID)
	if err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	if len(friends) == 0 {
		fmt.Fprintf(c.writer, "Client %s has no friends.\n", clientID[:8])
		return
	}

	fmt.Fprintf(c.writer, "\n┌─ Friends for %s ─────────────────────────\n", clientID[:8])
	for _, f := range friends {
		online := "offline"
		if f.Online {
			online = "online"
		}
		fmt.Fprintf(c.writer, "│ %s - %s (%s)\n", f.ClientID[:8], f.DisplayName, online)
	}
	fmt.Fprintf(c.writer, "└─ Total: %d ───────────────────────────────────────────\n", len(friends))
}

func (c *Console) addFriend(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(c.writer, "Usage: add-friend <clientId1> <clientId2>")
		return
	}

	clientID1 := c.hub.FindClientByPrefix(args[0])
	if clientID1 == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	clientID2 := c.hub.FindClientByPrefix(args[1])
	if clientID2 == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[1])
		return
	}

	if err := c.hub.CreateTestFriendship(clientID1, clientID2); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Added friendship: %s <-> %s\n", clientID1[:8], clientID2[:8])
}

func (c *Console) removeFriend(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(c.writer, "Usage: remove-friend <clientId1> <clientId2>")
		return
	}

	clientID1 := c.hub.FindClientByPrefix(args[0])
	if clientID1 == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	clientID2 := c.hub.FindClientByPrefix(args[1])
	if clientID2 == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[1])
		return
	}

	if err := c.hub.RemoveTestFriendship(clientID1, clientID2); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Removed friendship: %s <-> %s\n", clientID1[:8], clientID2[:8])
}

func (c *Console) sendPosition(args []string) {
	if len(args) < 5 {
		fmt.Fprintln(c.writer, "Usage: pos <clientId> <map> <x> <y> <z> [rotation]")
		fmt.Fprintln(c.writer, "Example: pos abc123 customs 100.5 50.2 -20.0 180")
		return
	}

	clientID := c.hub.FindClientByPrefix(args[0])
	if clientID == "" {
		fmt.Fprintf(c.writer, "Client not found: %s\n", args[0])
		return
	}

	mapName := args[1]

	x, err := strconv.ParseFloat(args[2], 64)
	if err != nil {
		fmt.Fprintf(c.writer, "Invalid X coordinate: %s\n", args[2])
		return
	}

	y, err := strconv.ParseFloat(args[3], 64)
	if err != nil {
		fmt.Fprintf(c.writer, "Invalid Y coordinate: %s\n", args[3])
		return
	}

	z, err := strconv.ParseFloat(args[4], 64)
	if err != nil {
		fmt.Fprintf(c.writer, "Invalid Z coordinate: %s\n", args[4])
		return
	}

	rotation := 0.0
	if len(args) > 5 {
		rotation, err = strconv.ParseFloat(args[5], 64)
		if err != nil {
			fmt.Fprintf(c.writer, "Invalid rotation: %s\n", args[5])
			return
		}
	}

	if err := c.hub.SendTestPosition(clientID, mapName, x, y, z, rotation); err != nil {
		fmt.Fprintf(c.writer, "Error: %v\n", err)
		return
	}

	fmt.Fprintf(c.writer, "Sent position for %s: %s (%.1f, %.1f, %.1f)\n", clientID[:8], mapName, x, y, z)
}

// StartBackground starts the console in a goroutine
func (c *Console) StartBackground() {
	log.Println("[Console] Starting interactive console...")
	go c.Run()
}
