package game

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type raidTestLogger struct{}

func (raidTestLogger) Debug(string)   {}
func (raidTestLogger) Info(string)    {}
func (raidTestLogger) Warning(string) {}
func (raidTestLogger) Error(string)   {}

func TestApplicationLogStartupOffsetDoesNotReplayHistoryAsLiveRaid(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "application_000.log")
	oldLog := "2026-10-08 12:00:00.000|1|Info|application|GameStarted: old raid\n"
	if err := os.WriteFile(logPath, []byte(oldLog), 0600); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(raidTestLogger{})
	defer watcher.Stop()
	if err := watcher.startLogFileWatcher(logPath, "application"); err != nil {
		t.Fatalf("start application log watcher: %v", err)
	}
	if got := watcher.GetRaidInfo(); got.StartedTime != nil {
		t.Fatalf("historical GameStarted was treated as a live raid: %+v", got)
	}

	stat, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	watcher.watcherMutex.Lock()
	position := watcher.filePositions[logPath]
	watcher.watcherMutex.Unlock()
	if position != stat.Size() {
		t.Fatalf("initial log offset = %d, want EOF %d", position, stat.Size())
	}

	liveLog := "2026-10-09 12:00:00.000|1|Info|application|GameStarted: current raid\n"
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(liveLog); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := watcher.checkForNewLogData(logPath, "application"); err != nil {
		t.Fatalf("read appended live log: %v", err)
	}
	if got := watcher.GetRaidInfo(); got.StartedTime == nil {
		t.Fatal("new GameStarted event was not detected")
	} else if !got.StartedTime.Equal(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("raid start time = %v, want timestamp from appended event", got.StartedTime)
	}
}

func TestWatcherFollowsNewTarkovLogSession(t *testing.T) {
	logsRoot := t.TempDir()
	oldFolder := filepath.Join(logsRoot, "log_2026.10.08_12-00-00_1.0.0")
	if err := os.Mkdir(oldFolder, 0700); err != nil {
		t.Fatal(err)
	}
	oldLogPath := filepath.Join(oldFolder, "application_000.log")
	if err := os.WriteFile(oldLogPath, nil, 0600); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(raidTestLogger{})
	defer watcher.Stop()
	watcher.logsPath = logsRoot
	if err := watcher.watchLatestLogFolder(); err != nil {
		t.Fatalf("watch first session: %v", err)
	}

	newFolder := filepath.Join(logsRoot, "log_2026.10.09_12-00-00_1.0.0")
	if err := os.Mkdir(newFolder, 0700); err != nil {
		t.Fatal(err)
	}
	newLogPath := filepath.Join(newFolder, "application_000.log")
	if err := os.WriteFile(newLogPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := watcher.watchLatestLogFolder(); err != nil {
		t.Fatalf("watch new session: %v", err)
	}
	outputLogPath := filepath.Join(newFolder, "output_000.log")
	if err := os.WriteFile(outputLogPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	// Rechecking the same session must discover files created after its folder.
	if err := watcher.watchLatestLogFolder(); err != nil {
		t.Fatalf("rescan current session: %v", err)
	}

	watcher.watcherMutex.Lock()
	defer watcher.watcherMutex.Unlock()
	if watcher.currentLogFolder != newFolder {
		t.Fatalf("current log folder = %q, want %q", watcher.currentLogFolder, newFolder)
	}
	if _, ok := watcher.filePositions[newLogPath]; !ok {
		t.Fatal("new session application log was not registered")
	}
	if _, ok := watcher.filePositions[outputLogPath]; !ok {
		t.Fatal("output log was not discovered on the current-session rescan")
	}
	if _, ok := watcher.filePositions[oldLogPath]; !ok {
		t.Fatal("previous session file tracking should remain available until shutdown")
	}
}

func TestWatcherStartsBeforeTarkovCreatesSessionFolder(t *testing.T) {
	logsRoot := t.TempDir()
	watcher := NewWatcher(raidTestLogger{})
	defer watcher.Stop()
	watcher.SetLogsPath(logsRoot)
	if err := watcher.Start(); err != nil {
		t.Fatalf("start watcher without an existing session: %v", err)
	}

	folder := filepath.Join(logsRoot, "log_2026.10.09_12-00-00_1.0.0")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(folder, "application_000.log")
	if err := os.WriteFile(logPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := watcher.watchLatestLogFolder(); err != nil {
		t.Fatalf("discover newly created Tarkov session: %v", err)
	}

	watcher.watcherMutex.Lock()
	defer watcher.watcherMutex.Unlock()
	if watcher.currentLogFolder != folder {
		t.Fatalf("current log folder = %q, want %q", watcher.currentLogFolder, folder)
	}
}

func TestRecentSessionReplayDetectsRaidAlreadyInProgress(t *testing.T) {
	logsRoot := t.TempDir()
	folder := filepath.Join(logsRoot, "log_"+time.Now().Format("2006.01.02_15-04-05")+"_1.0.0")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(folder, "application_000.log")
	line := "2026-10-09 12:00:00.000|1|Info|application|GameStarted: current raid\n"
	if err := os.WriteFile(logPath, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(raidTestLogger{})
	defer watcher.Stop()
	if err := watcher.startLogFileWatcher(logPath, "application"); err != nil {
		t.Fatalf("start current session watcher: %v", err)
	}
	if got := watcher.GetRaidInfo(); got.StartedTime == nil {
		t.Fatal("recent session replay did not detect the already-active raid")
	}
}

func TestRecentSessionReplaysOutputDisconnectBeforeApplicationState(t *testing.T) {
	logsRoot := t.TempDir()
	folder := filepath.Join(logsRoot, "log_"+time.Now().Format("2006.01.02_15-04-05")+"_1.0.0")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "application_000.log"), []byte("2026-10-09 12:00:00.000|1|Info|application|GameStarted: current raid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "output_000.log"), []byte("Session.OnDisconnect\n"), 0600); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(raidTestLogger{})
	defer watcher.Stop()
	watcher.logsPath = logsRoot
	if err := watcher.watchLatestLogFolder(); err != nil {
		t.Fatalf("watch recent session: %v", err)
	}
	if got := watcher.GetRaidInfo(); got.StartedTime == nil {
		t.Fatal("latest application GameStarted was not applied after output disconnect")
	}
}

func TestOutputLogDisconnectClearsRaidWithoutApplicationLogTimestamp(t *testing.T) {
	watcher := NewWatcher(raidTestLogger{})
	startedAt := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	watcher.processLogEvent("application|GameStarted:", startedAt, false)
	if got := watcher.GetRaidInfo(); got.StartedTime == nil {
		t.Fatal("test setup failed to start raid")
	}

	watcher.handleNewLogData("output_000.log", "output", "Session.OnDisconnect", false)
	if got := watcher.GetRaidInfo(); got.StartedTime != nil {
		t.Fatalf("raw output-log disconnect left raid active: %+v", got)
	}
}
