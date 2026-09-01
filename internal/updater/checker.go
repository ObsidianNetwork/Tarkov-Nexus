package updater

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
)

// BackgroundChecker performs periodic update checks
type BackgroundChecker struct {
	updater        *Updater
	logger         logger.Logger
	enabled        bool
	checkInterval  time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	isRunning      bool
	runningMutex   sync.RWMutex
	onUpdateFound  func(*UpdateInfo)
}

// NewBackgroundChecker creates a new background update checker
func NewBackgroundChecker(updater *Updater, log logger.Logger, checkInterval time.Duration) *BackgroundChecker {
	ctx, cancel := context.WithCancel(context.Background())

	// Guard against zero/negative interval which would panic time.NewTicker
	if checkInterval <= 0 {
		checkInterval = 24 * time.Hour
	}

	return &BackgroundChecker{
		updater:       updater,
		logger:        log,
		enabled:       true,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins periodic update checking
func (bc *BackgroundChecker) Start() {
	bc.runningMutex.Lock()
	if bc.isRunning {
		bc.runningMutex.Unlock()
		return
	}
	bc.isRunning = true
	bc.runningMutex.Unlock()

	bc.logger.Debug("Starting background update checker")

	// Perform initial check on startup
	go func() {
		time.Sleep(5 * time.Second) // Wait 5 seconds after startup
		bc.performCheck()
	}()

	// Start periodic checking
	go bc.checkLoop()
}

// Stop stops the background checker
func (bc *BackgroundChecker) Stop() {
	bc.runningMutex.Lock()
	defer bc.runningMutex.Unlock()

	if !bc.isRunning {
		return
	}

	bc.logger.Debug("Stopping background update checker")
	bc.cancel()
	bc.isRunning = false
}

// IsRunning returns true if the checker is running
func (bc *BackgroundChecker) IsRunning() bool {
	bc.runningMutex.RLock()
	defer bc.runningMutex.RUnlock()
	return bc.isRunning
}

// SetEnabled enables or disables background checking
func (bc *BackgroundChecker) SetEnabled(enabled bool) {
	bc.enabled = enabled
	bc.logger.Debug(fmt.Sprintf("Background update checking %s", map[bool]string{true: "enabled", false: "disabled"}[enabled]))
}

// SetCheckInterval updates the check interval
func (bc *BackgroundChecker) SetCheckInterval(interval time.Duration) {
	bc.checkInterval = interval
	bc.logger.Debug(fmt.Sprintf("Update check interval set to: %v", interval))
}

// OnUpdateFound sets a callback for when an update is found
func (bc *BackgroundChecker) OnUpdateFound(callback func(*UpdateInfo)) {
	bc.onUpdateFound = callback
}

// CheckNow performs an immediate update check
func (bc *BackgroundChecker) CheckNow() {
	bc.logger.Info("Performing manual update check")
	go bc.performCheck()
}

// checkLoop runs the periodic update checking loop
func (bc *BackgroundChecker) checkLoop() {
	ticker := time.NewTicker(bc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bc.ctx.Done():
			return
		case <-ticker.C:
			if bc.enabled {
				bc.performCheck()
			}
		}
	}
}

// performCheck executes a single update check
func (bc *BackgroundChecker) performCheck() {
	if !bc.enabled {
		return
	}

	bc.logger.Debug("Performing background update check")

	info, err := bc.updater.CheckForUpdates()
	if err != nil {
		bc.logger.Debug(fmt.Sprintf("Update check failed: %v", err))
		return
	}

	if info != nil {
		bc.logger.Info(fmt.Sprintf("Update found: %s", info.Version))
		if bc.onUpdateFound != nil {
			bc.onUpdateFound(info)
		}
	} else {
		bc.logger.Debug("No update available")
	}
}
