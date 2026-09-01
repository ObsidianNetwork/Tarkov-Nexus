package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/position"
)

// FileInfo represents information about a detected file
type FileInfo struct {
	Filepath  string    `json:"filepath"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"modTime"`
	EventType string    `json:"eventType"`
	Timestamp time.Time `json:"timestamp"`
}

// ScreenshotMonitor watches for new screenshots and processes them
type ScreenshotMonitor struct {
	screenshotDir   string
	logger          logger.Logger
	watcher         *fsnotify.Watcher
	processedFiles  map[string]time.Time // Changed to track timestamp for cleanup
	pendingFiles    map[string]*time.Timer
	fileTypes       []string
	debounceTime    time.Duration
	isMonitoring    bool
	ctx             context.Context
	cancel          context.CancelFunc
	eventHandlers   map[string][]func(interface{})
	handlerMutex    sync.RWMutex
	processMutex    sync.Mutex
}

// NewScreenshotMonitor creates a new screenshot monitor
func NewScreenshotMonitor(screenshotDir string, debounceTime time.Duration, log logger.Logger) *ScreenshotMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &ScreenshotMonitor{
		screenshotDir:  screenshotDir,
		logger:         log,
		processedFiles: make(map[string]time.Time),
		pendingFiles:   make(map[string]*time.Timer),
		fileTypes:      []string{".png", ".jpg", ".jpeg"},
		debounceTime:   debounceTime,
		ctx:            ctx,
		cancel:         cancel,
		eventHandlers:  make(map[string][]func(interface{})),
	}
}

// StartMonitoring begins watching the screenshot directory
func (sm *ScreenshotMonitor) StartMonitoring() error {
	if sm.isMonitoring {
		return fmt.Errorf("screenshot monitoring is already active")
	}

	if _, err := os.Stat(sm.screenshotDir); os.IsNotExist(err) {
		return fmt.Errorf("screenshot directory does not exist: %s", sm.screenshotDir)
	}

	sm.logger.Info(fmt.Sprintf("Starting screenshot monitoring: %s", sm.screenshotDir))

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	sm.watcher = watcher

	// Add directory to watcher
	if err := sm.watcher.Add(sm.screenshotDir); err != nil {
		sm.watcher.Close()
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	sm.isMonitoring = true
	sm.logger.Info("Screenshot monitoring is ready")
	sm.emit("ready", nil)

	// Start event processing
	go sm.processEvents()

	// Start cleanup goroutine to prevent memory leaks
	// Cleans up processed files older than 1 hour every 10 minutes
	go sm.runCleanupLoop()

	return nil
}

// runCleanupLoop periodically cleans up old processed file entries
func (sm *ScreenshotMonitor) runCleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.cleanupOldProcessedFiles(1 * time.Hour)
		}
	}
}

// StopMonitoring stops watching the screenshot directory
func (sm *ScreenshotMonitor) StopMonitoring() {
	if !sm.isMonitoring {
		return
	}

	sm.logger.Info("Stopping screenshot monitoring")

	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()

	// Clear any pending debounced operations
	for filepath, timer := range sm.pendingFiles {
		timer.Stop()
		delete(sm.pendingFiles, filepath)
	}

	if sm.watcher != nil {
		sm.watcher.Close()
		sm.watcher = nil
	}

	sm.cancel()
	sm.isMonitoring = false
	sm.emit("stopped", nil)
}

// processEvents handles file system events
func (sm *ScreenshotMonitor) processEvents() {
	defer sm.watcher.Close()

	for {
		select {
		case <-sm.ctx.Done():
			return

		case event, ok := <-sm.watcher.Events:
			if !ok {
				return
			}
			sm.handleFileEvent(event)

		case err, ok := <-sm.watcher.Errors:
			if !ok {
				return
			}
			sm.logger.Error(fmt.Sprintf("File watcher error: %v", err))
			sm.emit("error", err)
		}
	}
}

// handleFileEvent processes a single file system event
func (sm *ScreenshotMonitor) handleFileEvent(event fsnotify.Event) {
	filename := filepath.Base(event.Name)
	ext := strings.ToLower(filepath.Ext(filename))

	// Only process supported image files
	if !sm.isSupportedFileType(ext) {
		return
	}

	// Skip already processed files for CREATE events
	if event.Op&fsnotify.Create != 0 && sm.isFileProcessed(event.Name) {
		return
	}

	eventType := "unknown"
	if event.Op&fsnotify.Create != 0 {
		eventType = "create"
	} else if event.Op&fsnotify.Write != 0 {
		eventType = "write"
	}

	sm.logger.Debug(fmt.Sprintf("File %s: %s", eventType, filename))

	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()

	// Clear any existing timeout for this file
	if timer, exists := sm.pendingFiles[event.Name]; exists {
		timer.Stop()
		delete(sm.pendingFiles, event.Name)
	}

	// Set up debounced processing
	timer := time.AfterFunc(sm.debounceTime, func() {
		sm.processFile(event.Name, eventType)
		
		sm.processMutex.Lock()
		delete(sm.pendingFiles, event.Name)
		sm.processMutex.Unlock()
	})

	sm.pendingFiles[event.Name] = timer
}

// processFile processes a single file after debounce
func (sm *ScreenshotMonitor) processFile(filepath, eventType string) {
	// Check if file has already been processed
	sm.processMutex.Lock()
	if _, exists := sm.processedFiles[filepath]; exists {
		sm.processMutex.Unlock()
		sm.logger.Debug(fmt.Sprintf("File already processed, skipping: %s", filepath))
		return
	}
	sm.processMutex.Unlock()

	// Verify file still exists and is readable
	stat, err := os.Stat(filepath)
	if err != nil {
		sm.logger.Warning(fmt.Sprintf("File no longer exists or is not readable: %s", filepath))
		return
	}

	if !stat.Mode().IsRegular() {
		return
	}

	// Check file size - skip empty files
	if stat.Size() == 0 {
		sm.logger.Debug(fmt.Sprintf("Skipping empty file: %s", filepath))
		return
	}

	// Mark as processed
	sm.markFileProcessed(filepath)

	fileInfo := &FileInfo{
		Filepath:  filepath,
		Filename:  filepath,
		Size:      stat.Size(),
		ModTime:   stat.ModTime(),
		EventType: eventType,
		Timestamp: time.Now(),
	}

	sm.logger.Info(fmt.Sprintf("Processing screenshot: %s (%d bytes)", fileInfo.Filename, fileInfo.Size))

	// Emit events for different processing stages
	sm.emit("fileDetected", fileInfo)

	// Check if this looks like an Escape from Tarkov screenshot
	if position.IsEFTScreenshot(fileInfo.Filename) {
		sm.logger.Info(fmt.Sprintf("✅ Detected as EFT screenshot: %s", fileInfo.Filename))
		sm.emit("eftScreenshot", fileInfo)
	} else {
		sm.logger.Debug(fmt.Sprintf("❌ Not detected as EFT screenshot: %s", fileInfo.Filename))
		sm.emit("genericScreenshot", fileInfo)
	}
}

// ProcessExistingFiles processes existing files in the directory
func (sm *ScreenshotMonitor) ProcessExistingFiles() error {
	if _, err := os.Stat(sm.screenshotDir); os.IsNotExist(err) {
		return fmt.Errorf("screenshot directory does not exist: %s", sm.screenshotDir)
	}

	sm.logger.Info("Processing existing screenshots...")

	entries, err := os.ReadDir(sm.screenshotDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	imageFiles := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		ext := strings.ToLower(filepath.Ext(filename))
		if sm.isSupportedFileType(ext) {
			imageFiles = append(imageFiles, filename)
		}
	}

	sm.logger.Info(fmt.Sprintf("Found %d existing image files", len(imageFiles)))

	for _, filename := range imageFiles {
		filepath := filepath.Join(sm.screenshotDir, filename)

		if !sm.isFileProcessed(filepath) {
			sm.processFile(filepath, "existing")

			// Small delay between processing files to avoid overwhelming
			time.Sleep(50 * time.Millisecond)
		}
	}

	sm.logger.Info("Finished processing existing files")
	sm.emit("existingFilesProcessed", len(imageFiles))

	return nil
}

// OnEvent registers an event handler
func (sm *ScreenshotMonitor) OnEvent(event string, handler func(interface{})) {
	sm.handlerMutex.Lock()
	defer sm.handlerMutex.Unlock()

	sm.eventHandlers[event] = append(sm.eventHandlers[event], handler)
}

// emit triggers event handlers
func (sm *ScreenshotMonitor) emit(event string, data interface{}) {
	sm.handlerMutex.RLock()
	defer sm.handlerMutex.RUnlock()

	if handlers, exists := sm.eventHandlers[event]; exists {
		for _, handler := range handlers {
			go func(h func(interface{}), ev string) {
				defer func() {
					if r := recover(); r != nil {
						sm.logger.Error(fmt.Sprintf("Panic in screenshot monitor event handler [%s]: %v", ev, r))
					}
				}()
				h(data)
			}(handler, event)
		}
	}
}

// isSupportedFileType checks if the file extension is supported
func (sm *ScreenshotMonitor) isSupportedFileType(ext string) bool {
	for _, supportedExt := range sm.fileTypes {
		if ext == supportedExt {
			return true
		}
	}
	return false
}

// isFileProcessed checks if a file has already been processed
func (sm *ScreenshotMonitor) isFileProcessed(filepath string) bool {
	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()
	_, exists := sm.processedFiles[filepath]
	return exists
}

// markFileProcessed marks a file as processed with current timestamp
func (sm *ScreenshotMonitor) markFileProcessed(filepath string) {
	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()
	sm.processedFiles[filepath] = time.Now()
}

// GetStatus returns the current monitoring status
func (sm *ScreenshotMonitor) GetStatus() map[string]interface{} {
	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()

	return map[string]interface{}{
		"isMonitoring":    sm.isMonitoring,
		"directory":       sm.screenshotDir,
		"processedCount":  len(sm.processedFiles),
		"pendingCount":    len(sm.pendingFiles),
		"watcherActive":   sm.watcher != nil,
	}
}

// ClearProcessedHistory clears the processed file history
func (sm *ScreenshotMonitor) ClearProcessedHistory() {
	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()

	sm.processedFiles = make(map[string]time.Time)
	sm.logger.Info("Cleared processed file history")
}

// cleanupOldProcessedFiles removes entries older than the specified duration
// This prevents unbounded memory growth over long running sessions
func (sm *ScreenshotMonitor) cleanupOldProcessedFiles(maxAge time.Duration) {
	sm.processMutex.Lock()
	defer sm.processMutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for filepath, processedAt := range sm.processedFiles {
		if processedAt.Before(cutoff) {
			delete(sm.processedFiles, filepath)
			removed++
		}
	}
	if removed > 0 {
		sm.logger.Debug(fmt.Sprintf("Cleaned up %d old processed file entries", removed))
	}
}

// IsMonitoring returns true if monitoring is active
func (sm *ScreenshotMonitor) IsMonitoring() bool {
	return sm.isMonitoring
}