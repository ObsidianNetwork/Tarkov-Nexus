package autoscreenshot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/config"
	"tarkov-screenshot-analyzer/internal/logger"
)

const defaultPollInterval = 500 * time.Millisecond

const (
	StateDisabled        = "disabled"
	StateUnsupported     = "unsupported"
	StateWaitingForGame  = "waiting_for_game"
	StateWaitingForRaid  = "waiting_for_raid"
	StateWaitingForFocus = "waiting_for_focus"
	StateCountdown       = "countdown"
	StateSending         = "sending"
	StateError           = "error"
)

// Driver performs the small set of Windows operations required by automatic
// screenshots. Input is only sent after the foreground check succeeds.
type Driver interface {
	GameRunning() (bool, error)
	GameForeground() (bool, error)
	TapKey(vk uint16) error
}

type RaidState func() bool

type Status struct {
	Enabled        bool      `json:"enabled"`
	State          string    `json:"state"`
	LastKeySent    time.Time `json:"lastKeySent,omitempty"`
	Error          string    `json:"error,omitempty"`
	IntervalSecond int       `json:"intervalSeconds"`
	KeyCode        int       `json:"keyCode"`
}

// Controller schedules in-game screenshot key taps while all safety gates are
// satisfied. It never captures or creates image files itself.
type Controller struct {
	mu         sync.RWMutex
	actionMu   sync.Mutex
	cfg        config.Config
	driver     Driver
	raidLive   RaidState
	logger     logger.Logger
	status     Status
	pollEvery  time.Duration
	cancel     context.CancelFunc
	done       chan struct{}
	generation uint64
}

func New(cfg config.Config, driver Driver, raidLive RaidState, log logger.Logger) *Controller {
	cfg.NormalizeAutoScreenshotSettings()
	initial := StateDisabled
	if cfg.AutoScreenshot {
		initial = StateWaitingForGame
		if driver == nil {
			initial = StateUnsupported
		}
	}
	return &Controller{
		cfg:       cfg,
		driver:    driver,
		raidLive:  raidLive,
		logger:    log,
		pollEvery: defaultPollInterval,
		status: Status{
			Enabled:        cfg.AutoScreenshot,
			State:          initial,
			IntervalSecond: cfg.ScreenshotInterval,
			KeyCode:        cfg.ScreenshotKey,
		},
	}
}

func (c *Controller) Start(parent context.Context) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()
	go c.run(ctx, done)
}

func (c *Controller) Stop() {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
		c.mu.Lock()
		if c.done == done {
			c.cancel = nil
			c.done = nil
		}
		c.mu.Unlock()
	}
}

// UpdateConfig applies the small subset of settings consumed by the worker.
func (c *Controller) UpdateConfig(cfg config.Config) {
	cfg.NormalizeAutoScreenshotSettings()
	// Serialize setting changes with the final input operation. Once this
	// method returns, no key can be sent using the previous configuration.
	c.actionMu.Lock()
	defer c.actionMu.Unlock()
	c.mu.Lock()
	if c.cfg.AutoScreenshot != cfg.AutoScreenshot || c.cfg.ScreenshotKey != cfg.ScreenshotKey || c.cfg.ScreenshotInterval != cfg.ScreenshotInterval {
		c.generation++
	}
	c.cfg.AutoScreenshot = cfg.AutoScreenshot
	c.cfg.ScreenshotKey = cfg.ScreenshotKey
	c.cfg.ScreenshotInterval = cfg.ScreenshotInterval
	c.status.Enabled = cfg.AutoScreenshot
	c.status.IntervalSecond = cfg.ScreenshotInterval
	c.status.KeyCode = cfg.ScreenshotKey
	if !cfg.AutoScreenshot {
		c.status.State = StateDisabled
		c.status.Error = ""
	} else if c.driver == nil {
		c.status.State = StateUnsupported
	} else if c.status.State == StateDisabled || c.status.State == StateUnsupported {
		c.status.State = StateWaitingForGame
	}
	c.mu.Unlock()
}

func (c *Controller) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Controller) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(c.pollEvery)
	defer ticker.Stop()
	var eligibleSince time.Time
	var generation uint64
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cfg, currentGeneration := c.currentConfig()
			if currentGeneration != generation {
				eligibleSince = time.Time{}
				generation = currentGeneration
			}
			if !cfg.AutoScreenshot {
				eligibleSince = time.Time{}
				c.setState(StateDisabled, "")
				continue
			}
			if c.driver == nil {
				eligibleSince = time.Time{}
				c.setState(StateUnsupported, "Automatic screenshots are only supported on Windows.")
				continue
			}
			running, err := c.driver.GameRunning()
			if err != nil {
				eligibleSince = time.Time{}
				c.setState(StateError, fmt.Sprintf("Could not check whether Tarkov is running: %v", err))
				continue
			}
			if !running {
				eligibleSince = time.Time{}
				c.setState(StateWaitingForGame, "")
				continue
			}
			if c.raidLive == nil || !c.raidLive() {
				eligibleSince = time.Time{}
				c.setState(StateWaitingForRaid, "")
				continue
			}
			foreground, err := c.driver.GameForeground()
			if err != nil {
				eligibleSince = time.Time{}
				c.setState(StateError, fmt.Sprintf("Could not check Tarkov focus: %v", err))
				continue
			}
			if !foreground {
				eligibleSince = time.Time{}
				c.setState(StateWaitingForFocus, "")
				continue
			}
			if eligibleSince.IsZero() {
				eligibleSince = now
			}
			interval := time.Duration(cfg.ScreenshotInterval) * time.Second
			if now.Sub(eligibleSince) < interval {
				c.setState(StateCountdown, "")
				continue
			}
			// Serialize the last eligibility checks and input with config updates.
			c.actionMu.Lock()
			// Recheck the active raid and focus immediately before producing input.
			latest, latestGeneration := c.currentConfig()
			if latestGeneration != currentGeneration || !latest.AutoScreenshot {
				eligibleSince = time.Time{}
				c.actionMu.Unlock()
				continue
			}
			if c.raidLive == nil || !c.raidLive() {
				eligibleSince = time.Time{}
				c.setState(StateWaitingForRaid, "")
				c.actionMu.Unlock()
				continue
			}
			foreground, err = c.driver.GameForeground()
			if err != nil || !foreground {
				eligibleSince = time.Time{}
				if err != nil {
					c.setState(StateError, fmt.Sprintf("Could not recheck Tarkov focus: %v", err))
				} else {
					c.setState(StateWaitingForFocus, "")
				}
				c.actionMu.Unlock()
				continue
			}
			c.setState(StateSending, "")
			if err := c.driver.TapKey(uint16(cfg.ScreenshotKey)); err != nil {
				eligibleSince = time.Time{}
				message := fmt.Sprintf("Could not send screenshot key (Windows may block input if Tarkov is running elevated): %v", err)
				c.setState(StateError, message)
				c.actionMu.Unlock()
				continue
			}
			c.actionMu.Unlock()
			eligibleSince = now
			c.setLastKeySent(now)
			c.clearError()
			c.setState(StateCountdown, "")
			if c.logger != nil {
				c.logger.Debug("Automatic in-game screenshot key sent")
			}
		}
	}
}

func (c *Controller) currentConfig() (config.Config, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg, c.generation
}

func (c *Controller) setState(state, message string) {
	c.mu.Lock()
	changed := c.status.State != state
	c.status.State = state
	if message != "" {
		changed = changed || c.status.Error != message
		c.status.Error = message
	}
	c.mu.Unlock()
	if changed && c.logger != nil && message != "" {
		c.logger.Warning(message)
	}
}

func (c *Controller) clearError() {
	c.mu.Lock()
	c.status.Error = ""
	c.mu.Unlock()
}

func (c *Controller) setLastKeySent(at time.Time) {
	c.mu.Lock()
	c.status.LastKeySent = at
	c.mu.Unlock()
}
