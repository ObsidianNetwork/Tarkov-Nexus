package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const defaultBaseURL = "http://localhost:8765"

var baseURL = defaultBaseURL

func main() {
	// Check for custom URL
	if url := os.Getenv("PARTY_SERVER_URL"); url != "" {
		baseURL = url
	}

	if len(os.Args) > 1 {
		// Command mode: party-cli create-client Alice
		handleCommand(os.Args[1:])
	} else {
		// Interactive mode
		runInteractive()
	}
}

func runInteractive() {
	printBanner()
	printHelp()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\n> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		args := strings.Fields(input)
		if len(args) == 0 {
			continue
		}

		if args[0] == "exit" || args[0] == "quit" || args[0] == "q" {
			fmt.Println("Goodbye!")
			return
		}

		handleCommand(args)
	}
}

func printBanner() {
	fmt.Println(`
╔═══════════════════════════════════════════════════════════╗
║          TARKOV PARTY SERVER - DEV CLI                    ║
║                                                           ║
║  Type 'help' for available commands                       ║
╚═══════════════════════════════════════════════════════════╝`)
}

func printHelp() {
	fmt.Println(`
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
  delete-party <code>           Delete a party

FRIEND MANAGEMENT:
  friends <clientId>            List friends for a client
  add-friend <id1> <id2>        Make two clients friends
  remove-friend <id1> <id2>     Remove friendship

POSITION TESTING:
  pos <clientId> <map> <x> <y> <z> [rotation]
                                Send test position update

UTILITIES:
  clear                         Clear the console
  exit, quit                    Exit CLI
───────────────────────────────────────────────────────────`)
}

func handleCommand(args []string) {
	cmd := strings.ToLower(args[0])
	cmdArgs := args[1:]

	switch cmd {
	case "help", "h", "?":
		printHelp()

	case "status":
		showStatus()

	case "clients", "list-clients", "lc":
		listClients()

	case "create-client", "cc":
		createClient(cmdArgs)

	case "remove-client", "rc", "delete-client", "dc":
		removeClient(cmdArgs)

	case "cleanup":
		cleanup()

	case "parties", "list-parties", "lp":
		listParties()

	case "create-party", "cp":
		createParty(cmdArgs)

	case "delete-party", "dp":
		deleteParty(cmdArgs)

	case "friends", "list-friends", "lf":
		listFriends(cmdArgs)

	case "add-friend", "af":
		addFriend(cmdArgs)

	case "remove-friend", "rf":
		removeFriend(cmdArgs)

	case "pos", "position":
		sendPosition(cmdArgs)

	case "clear", "cls":
		fmt.Print("\033[2J\033[H")
		printBanner()

	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

func showStatus() {
	resp, err := http.Get(baseURL + "/stats")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("\n┌─ Server Status ─────────────────────────────────────")
	fmt.Printf("│ Connected Clients: %.0f\n", data["clients"])
	fmt.Printf("│ Active Parties:    %.0f\n", data["parties"])
	if tc, ok := data["testClients"]; ok {
		fmt.Printf("│ Test Clients:      %.0f\n", tc)
	}
	fmt.Println("└─────────────────────────────────────────────────────")
}

func listClients() {
	resp, err := http.Get(baseURL + "/admin/clients")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data struct {
		Count   int `json:"count"`
		Clients []struct {
			ID          string `json:"clientId"`
			DisplayName string `json:"displayName"`
			PartyCode   string `json:"partyCode"`
			IsTest      bool   `json:"isTest"`
		} `json:"clients"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if data.Count == 0 {
		fmt.Println("\nNo connected clients.")
		return
	}

	fmt.Println("\n┌─ Connected Clients ─────────────────────────────────")
	for _, c := range data.Clients {
		test := ""
		if c.IsTest {
			test = " [TEST]"
		}
		party := ""
		if c.PartyCode != "" {
			party = fmt.Sprintf(" (party: %s)", c.PartyCode)
		}
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		fmt.Printf("│ %s - %s%s%s\n", id, c.DisplayName, party, test)
	}
	fmt.Printf("└─ Total: %d ───────────────────────────────────────────\n", data.Count)
}

func createClient(args []string) {
	name := ""
	if len(args) > 0 {
		name = strings.Join(args, " ")
	}

	body, _ := json.Marshal(map[string]string{"displayName": name})
	resp, err := http.Post(baseURL+"/admin/test-client", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: %s\n", data["error"])
		return
	}

	fmt.Println("Created test client:")
	fmt.Printf("  ID:   %s\n", data["clientId"])
	fmt.Printf("  Name: %s\n", data["displayName"])
}

func removeClient(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: remove-client <clientId>")
		return
	}

	req, _ := http.NewRequest("DELETE", baseURL+"/admin/test-client?clientId="+args[0], nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Removed client: %s\n", args[0])
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
	}
}

func cleanup() {
	resp, err := http.Post(baseURL+"/admin/cleanup", "application/json", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	fmt.Printf("Cleaned up %.0f test clients\n", data["clientsRemoved"])
}

func listParties() {
	resp, err := http.Get(baseURL + "/admin/parties")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data struct {
		Count   int `json:"count"`
		Parties []struct {
			Code        string `json:"code"`
			HostID      string `json:"hostId"`
			MemberCount int    `json:"memberCount"`
			Members     []struct {
				ID          string `json:"clientId"`
				DisplayName string `json:"displayName"`
			} `json:"members"`
		} `json:"parties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if data.Count == 0 {
		fmt.Println("\nNo active parties.")
		return
	}

	fmt.Println("\n┌─ Active Parties ────────────────────────────────────")
	for _, p := range data.Parties {
		hostID := p.HostID
		if len(hostID) > 8 {
			hostID = hostID[:8]
		}
		fmt.Printf("│ %s (host: %s, members: %d)\n", p.Code, hostID, p.MemberCount)
		for _, m := range p.Members {
			id := m.ID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Printf("│   └─ %s - %s\n", id, m.DisplayName)
		}
	}
	fmt.Printf("└─ Total: %d ───────────────────────────────────────────\n", data.Count)
}

func createParty(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: create-party <clientId>")
		return
	}

	body, _ := json.Marshal(map[string]string{"hostClientId": args[0]})
	resp, err := http.Post(baseURL+"/admin/test-party", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
		return
	}

	fmt.Printf("Created party: %s\n", data["partyCode"])
}

func deleteParty(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: delete-party <partyCode>")
		return
	}

	req, _ := http.NewRequest("DELETE", baseURL+"/admin/test-party?partyCode="+args[0], nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Deleted party: %s\n", args[0])
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
	}
}

func listFriends(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: friends <clientId>")
		return
	}
	// For now, just show a message - would need a dedicated endpoint
	fmt.Println("Use the clients command to see connected clients and their IDs")
}

func addFriend(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: add-friend <clientId1> <clientId2>")
		return
	}

	body, _ := json.Marshal(map[string]string{
		"clientId1": args[0],
		"clientId2": args[1],
	})
	resp, err := http.Post(baseURL+"/admin/test-friends", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Added friendship: %s <-> %s\n", args[0], args[1])
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
	}
}

func removeFriend(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: remove-friend <clientId1> <clientId2>")
		return
	}

	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("%s/admin/test-friends?clientId1=%s&clientId2=%s", baseURL, args[0], args[1]), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Removed friendship: %s <-> %s\n", args[0], args[1])
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
	}
}

func sendPosition(args []string) {
	if len(args) < 5 {
		fmt.Println("Usage: pos <clientId> <map> <x> <y> <z> [rotation]")
		return
	}

	x, _ := strconv.ParseFloat(args[2], 64)
	y, _ := strconv.ParseFloat(args[3], 64)
	z, _ := strconv.ParseFloat(args[4], 64)
	rotation := 0.0
	if len(args) > 5 {
		rotation, _ = strconv.ParseFloat(args[5], 64)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"clientId": args[0],
		"map":      args[1],
		"x":        x,
		"y":        y,
		"z":        z,
		"rotation": rotation,
	})
	resp, err := http.Post(baseURL+"/admin/test-position", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Sent position for %s: %s (%.1f, %.1f, %.1f)\n", args[0], args[1], x, y, z)
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: %s\n", string(body))
	}
}
