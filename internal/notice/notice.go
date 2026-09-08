// Package notice fetches maintainer-published status notices that are shown
// in the app UI. Notices let the maintainer reach users ("this is broken, a
// fix is coming") without shipping a release: edit notices/status.json on the
// repository's main branch and every running app picks it up within its
// fetch interval.
package notice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
)

const (
	// NoticeURL is the location of the maintainer-edited notice file.
	NoticeURL = "https://raw.githubusercontent.com/ObsidianNetwork/Tarkov-Nexus/main/notices/status.json"

	// DefaultInterval satisfies the "visible within one hour" product metric
	// while staying a trivial request volume against the CDN.
	DefaultInterval = 30 * time.Minute

	fetchTimeout = 15 * time.Second
	bodyLimit    = 1 << 20
)

// Severity of a notice; drives how prominently the UI renders it.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Notice is one maintainer-published status message.
type Notice struct {
	ID       string     `json:"id"`
	Severity Severity   `json:"severity"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Link     string     `json:"link,omitempty"`
	LinkText string     `json:"linkText,omitempty"`
	Expires  *time.Time `json:"expires,omitempty"`
}

// statusFile mirrors the shape of notices/status.json.
type statusFile struct {
	Notices []Notice `json:"notices"`
}

// Fetcher periodically fetches the notice file and keeps the last known
// good set. On fetch or parse failure it keeps serving the previous set —
// a broken notice file must never blank out a critical message.
type Fetcher struct {
	url        string
	interval   time.Duration
	logger     logger.Logger
	httpClient *http.Client

	mu   sync.Mutex
	last []Notice

	onUpdateMu sync.RWMutex
	onUpdate   func([]Notice)

	ctx    context.Context
	cancel context.CancelFunc
}

// NewFetcher creates a fetcher; call Start to begin polling.
func NewFetcher(url string, interval time.Duration, log logger.Logger) *Fetcher {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Fetcher{
		url:        url,
		interval:   interval,
		logger:     log,
		httpClient: &http.Client{Timeout: fetchTimeout},
		ctx:        ctx,
		cancel:     cancel,
	}
}

// OnUpdate registers the callback invoked after every successful fetch with
// the currently active (non-expired) notices.
func (f *Fetcher) OnUpdate(cb func([]Notice)) {
	f.onUpdateMu.Lock()
	defer f.onUpdateMu.Unlock()
	f.onUpdate = cb
}

// Start performs an immediate fetch and then polls on the configured interval.
func (f *Fetcher) Start() {
	f.logger.Debug("Starting notice fetcher")

	go func() {
		if _, err := f.FetchNow(); err != nil {
			f.logger.Debug(fmt.Sprintf("Initial notice fetch failed: %v", err))
		}
	}()

	go func() {
		ticker := time.NewTicker(f.interval)
		defer ticker.Stop()
		for {
			select {
			case <-f.ctx.Done():
				return
			case <-ticker.C:
				if _, err := f.FetchNow(); err != nil {
					f.logger.Debug(fmt.Sprintf("Notice fetch failed: %v", err))
				}
			}
		}
	}()
}

// Stop stops the polling loop.
func (f *Fetcher) Stop() {
	f.cancel()
}

// Last returns the most recent non-expired notice set (may be nil).
func (f *Fetcher) Last() []Notice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Active(f.last, time.Now())
}

// FetchNow fetches and parses the notice file, caches the result on success,
// and invokes the OnUpdate callback with the active set.
func (f *Fetcher) FetchNow() ([]Notice, error) {
	notices, err := f.fetch()
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.last = notices
	f.mu.Unlock()

	f.onUpdateMu.RLock()
	cb := f.onUpdate
	f.onUpdateMu.RUnlock()
	if cb != nil {
		cb(Active(notices, time.Now()))
	}
	return notices, nil
}

func (f *Fetcher) fetch() ([]Notice, error) {
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch notices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch notices: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read notices: %w", err)
	}

	var file statusFile
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("parse notices: %w", err)
	}

	// Boundary validation: drop notices the UI could not render meaningfully.
	valid := make([]Notice, 0, len(file.Notices))
	for _, n := range file.Notices {
		if n.ID == "" || n.Title == "" {
			continue
		}
		if n.Severity == "" {
			n.Severity = SeverityInfo
		}
		valid = append(valid, n)
	}
	return valid, nil
}

// Active filters out notices whose expiry has passed.
func Active(notices []Notice, now time.Time) []Notice {
	active := make([]Notice, 0, len(notices))
	for _, n := range notices {
		if n.Expires != nil && now.After(*n.Expires) {
			continue
		}
		active = append(active, n)
	}
	return active
}
