package autoscreenshot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/internal/config"
)

type fakeDriver struct {
	mu         sync.Mutex
	running    bool
	foreground bool
	err        error
	keys       []uint16
}

func (d *fakeDriver) GameRunning() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running, d.err
}

func (d *fakeDriver) GameForeground() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.foreground, d.err
}

func (d *fakeDriver) TapKey(key uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.keys = append(d.keys, key)
	return nil
}

func (d *fakeDriver) keyCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.keys)
}

func testController(t *testing.T, cfg config.Config, d *fakeDriver, raid *atomic.Bool) (*Controller, context.CancelFunc) {
	t.Helper()
	c := New(cfg, d, func() bool { return raid.Load() }, nil)
	c.pollEvery = 2 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	t.Cleanup(func() {
		cancel()
		c.Stop()
	})
	return c, cancel
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestControllerRequiresGameRaidAndForeground(t *testing.T) {
	cfg := config.Config{AutoScreenshot: true, ScreenshotKey: 44, ScreenshotInterval: 1}
	d := &fakeDriver{}
	var raid atomic.Bool
	c, _ := testController(t, cfg, d, &raid)

	waitFor(t, func() bool { return c.Status().State == StateWaitingForGame })
	if d.keyCount() != 0 {
		t.Fatal("sent key while game was absent")
	}

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()
	waitFor(t, func() bool { return c.Status().State == StateWaitingForRaid })
	if d.keyCount() != 0 {
		t.Fatal("sent key without active raid")
	}

	raid.Store(true)
	waitFor(t, func() bool { return c.Status().State == StateWaitingForFocus })
	if d.keyCount() != 0 {
		t.Fatal("sent key while game was not foreground")
	}

	d.mu.Lock()
	d.foreground = true
	d.mu.Unlock()
	waitFor(t, func() bool { return d.keyCount() == 1 })
	d.mu.Lock()
	got := d.keys[0]
	d.mu.Unlock()
	if got != 44 {
		t.Fatalf("sent key %d, want configured key 44", got)
	}
}

func TestControllerWaitsFullIntervalAndResetsOnFocusLoss(t *testing.T) {
	cfg := config.Config{AutoScreenshot: true, ScreenshotKey: 44, ScreenshotInterval: 1}
	d := &fakeDriver{running: true, foreground: true}
	var raid atomic.Bool
	raid.Store(true)
	c, _ := testController(t, cfg, d, &raid)

	waitFor(t, func() bool { return c.Status().State == StateCountdown })
	time.Sleep(50 * time.Millisecond)
	if d.keyCount() != 0 {
		t.Fatal("sent key before the configured interval")
	}
	waitFor(t, func() bool { return d.keyCount() == 1 })

	d.mu.Lock()
	d.foreground = false
	d.mu.Unlock()
	waitFor(t, func() bool { return c.Status().State == StateWaitingForFocus })
	d.mu.Lock()
	d.foreground = true
	d.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	if d.keyCount() != 1 {
		t.Fatal("sent immediately after focus returned")
	}
}

func TestControllerUpdateDisablesAndStopPreventsFurtherInput(t *testing.T) {
	cfg := config.Config{AutoScreenshot: true, ScreenshotKey: 44, ScreenshotInterval: 1}
	d := &fakeDriver{running: true, foreground: true}
	var raid atomic.Bool
	raid.Store(true)
	c, cancel := testController(t, cfg, d, &raid)
	waitFor(t, func() bool { return c.Status().State == StateCountdown })
	cfg.AutoScreenshot = false
	c.UpdateConfig(cfg)
	waitFor(t, func() bool { return c.Status().State == StateDisabled })
	time.Sleep(10 * time.Millisecond)
	if d.keyCount() != 0 {
		t.Fatal("sent key after automation was disabled")
	}
	cancel()
	c.Stop()
	c.Start(context.Background())
	c.Stop()
	if d.keyCount() != 0 {
		t.Fatal("sent key after stop/restart with automation disabled")
	}
}

func TestControllerReportsDriverErrors(t *testing.T) {
	cfg := config.Config{AutoScreenshot: true, ScreenshotKey: 44, ScreenshotInterval: 1}
	d := &fakeDriver{err: errors.New("access denied")}
	var raid atomic.Bool
	raid.Store(true)
	c, _ := testController(t, cfg, d, &raid)
	waitFor(t, func() bool { return c.Status().State == StateError })
	if c.Status().Error == "" {
		t.Fatal("expected actionable driver error")
	}
}
