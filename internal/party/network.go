package party

import (
	"fmt"
	"net"
	"strings"
)

// GetLocalIPAddresses returns all non-loopback IPv4 addresses for the local machine
func GetLocalIPAddresses() ([]string, error) {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, addr := range addrs {
		// Check if it's an IP network address
		if ipnet, ok := addr.(*net.IPNet); ok {
			// Skip loopback addresses
			if ipnet.IP.IsLoopback() {
				continue
			}

			// Only include IPv4 addresses
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}

	return ips, nil
}

// GetPreferredIP returns the most likely LAN IP address for party hosting
// Prioritizes private network ranges (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
func GetPreferredIP() (string, error) {
	ips, err := GetLocalIPAddresses()
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no network interfaces found")
	}

	// Priority order: 192.168.x.x > 10.x.x.x > 172.16-31.x.x > other
	var class192, class10, class172, other string

	for _, ip := range ips {
		if strings.HasPrefix(ip, "192.168.") {
			class192 = ip
		} else if strings.HasPrefix(ip, "10.") {
			class10 = ip
		} else if strings.HasPrefix(ip, "172.") {
			// Check if it's in the 172.16.0.0 - 172.31.255.255 range
			parts := strings.Split(ip, ".")
			if len(parts) >= 2 {
				var second int
				fmt.Sscanf(parts[1], "%d", &second)
				if second >= 16 && second <= 31 {
					class172 = ip
				}
			}
		} else {
			other = ip
		}
	}

	// Return in priority order
	if class192 != "" {
		return class192, nil
	}
	if class10 != "" {
		return class10, nil
	}
	if class172 != "" {
		return class172, nil
	}
	if other != "" {
		return other, nil
	}

	// Fallback to first IP
	return ips[0], nil
}

// IsPortAvailable checks if a TCP port is available for use
func IsPortAvailable(port int) bool {
	// Try to listen on the port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	// Port is available, close the listener
	listener.Close()
	return true
}

// GetWebSocketURLs returns all possible WebSocket URLs for the party server
func GetWebSocketURLs(port int) ([]string, error) {
	ips, err := GetLocalIPAddresses()
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(ips))
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("ws://%s:%d/party", ip, port))
	}

	return urls, nil
}

// GetPreferredWebSocketURL returns the most likely WebSocket URL for LAN use
func GetPreferredWebSocketURL(port int) (string, error) {
	ip, err := GetPreferredIP()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("ws://%s:%d/party", ip, port), nil
}
