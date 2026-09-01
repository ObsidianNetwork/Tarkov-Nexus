package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles communication with TarkovTracker API
type Client struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	rateLimiter *rateLimiter
}

// Config represents TarkovTracker client configuration
type Config struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
}

// TokenResponse represents the response from token validation
type TokenResponse struct {
	Token       string   `json:"token"`
	Permissions []string `json:"permissions"`
}

// TaskStatusUpdate represents the request body for task status updates
type TaskStatusUpdate struct {
	State string `json:"state"`
}

// TaskStatusBatchUpdate represents a batch task status update
type TaskStatusBatchUpdate struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// ProgressResponse represents the user's progress data
type ProgressResponse struct {
	Data ProgressData `json:"data"`
	Meta ProgressMeta `json:"meta"`
}

// ProgressData contains the user's progress information
type ProgressData struct {
	TasksProgress []TaskProgress `json:"tasksProgress"`
	UserID        string         `json:"userId"`
	DisplayName   string         `json:"displayName"`
	PlayerLevel   int            `json:"playerLevel"`
	GameEdition   int            `json:"gameEdition"`
	PmcFaction    string         `json:"pmcFaction"`
}

// ProgressMeta contains metadata about the progress response
type ProgressMeta struct {
	Self string `json:"self"`
}

// TaskProgress represents the progress of a specific task
type TaskProgress struct {
	ID       string `json:"id"`
	Complete bool   `json:"complete"`
	Failed   bool   `json:"failed"`
	Invalid  bool   `json:"invalid"`
}

// rateLimiter implements rate limiting for API requests
type rateLimiter struct {
	requests chan struct{}
	ticker   *time.Ticker
}

// newRateLimiter creates a rate limiter allowing 15 requests per minute
func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		requests: make(chan struct{}, 15), // Allow 15 requests in buffer
		ticker:   time.NewTicker(time.Minute / 15), // Refill every 4 seconds
	}

	// Fill initial buffer
	for i := 0; i < 15; i++ {
		rl.requests <- struct{}{}
	}

	// Start refill goroutine
	go rl.refill()

	return rl
}

// refill adds tokens to the rate limiter
func (rl *rateLimiter) refill() {
	for range rl.ticker.C {
		select {
		case rl.requests <- struct{}{}:
		default:
			// Buffer is full, skip
		}
	}
}

// wait blocks until a request token is available
func (rl *rateLimiter) wait() {
	<-rl.requests
}

// stop shuts down the rate limiter
func (rl *rateLimiter) stop() {
	rl.ticker.Stop()
}

// NewClient creates a new TarkovTracker API client
func NewClient(config Config) *Client {
	return &Client{
		baseURL: config.BaseURL,
		token:   config.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: newRateLimiter(),
	}
}

// Close shuts down the client and releases resources
func (c *Client) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.stop()
	}
}

// makeRequest performs an HTTP request with rate limiting and authentication
func (c *Client) makeRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	return c.makeRequestWithRateLimit(method, endpoint, body, true)
}

// makeRequestWithRateLimit performs an HTTP request with optional rate limiting
func (c *Client) makeRequestWithRateLimit(method, endpoint string, body interface{}, useRateLimit bool) (*http.Response, error) {
	// Rate limit the request if enabled
	if useRateLimit {
		c.rateLimiter.wait()
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TarkovMap-Go/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// TestToken validates the API token
func (c *Client) TestToken() (*TokenResponse, error) {
	resp, err := c.makeRequest("GET", "/token", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token validation failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetProgress retrieves the user's current progress for the specified game mode
func (c *Client) GetProgress(gameMode string) (*ProgressResponse, error) {
	// Validate gameMode
	if gameMode != "pvp" && gameMode != "pve" {
		gameMode = "pvp" // Default to pvp if invalid
	}

	endpoint := fmt.Sprintf("/progress?gameMode=%s", gameMode)
	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get progress with status %d", resp.StatusCode)
	}

	var progressResp ProgressResponse
	if err := json.NewDecoder(resp.Body).Decode(&progressResp); err != nil {
		return nil, fmt.Errorf("failed to decode progress response: %w", err)
	}

	return &progressResp, nil
}

// UpdateTaskStatus updates the status of a single task
func (c *Client) UpdateTaskStatus(taskID, status string) error {
	return c.UpdateTaskStatusWithRateLimit(taskID, status, true)
}

// UpdateTaskStatusWithRateLimit updates the status of a single task with optional rate limiting
func (c *Client) UpdateTaskStatusWithRateLimit(taskID, status string, useRateLimit bool) error {
	endpoint := fmt.Sprintf("/progress/task/%s", taskID)
	body := TaskStatusUpdate{State: status}

	resp, err := c.makeRequestWithRateLimit("POST", endpoint, body, useRateLimit)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read response body for error details
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update task status with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UpdateTaskStatuses updates multiple task statuses in batch
func (c *Client) UpdateTaskStatuses(updates []TaskStatusBatchUpdate) error {
	resp, err := c.makeRequest("POST", "/progress/tasks", updates)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update task statuses with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// IsValidToken checks if the token has the required permissions
func (c *Client) IsValidToken() (bool, error) {
	tokenResp, err := c.TestToken()
	if err != nil {
		return false, err
	}

	// Check for Write Progression permission
	for _, perm := range tokenResp.Permissions {
		if perm == "WP" {
			return true, nil
		}
	}

	return false, fmt.Errorf("token does not have Write Progression (WP) permission")
}

// GetConnectionStats returns connection statistics
func (c *Client) GetConnectionStats() map[string]interface{} {
	return map[string]interface{}{
		"baseURL":    c.baseURL,
		"hasToken":   c.token != "",
		"configured": c.token != "" && c.baseURL != "",
	}
}