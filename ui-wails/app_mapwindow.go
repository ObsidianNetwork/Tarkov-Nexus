package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wailsapp/wails/v2"

	"tarkov-screenshot-analyzer/internal/mapwindow"
)

//go:embed mapwindow_assets
var mapWindowAssets embed.FS

// mapWindowClass is the Win32 window class the map window registers under.
// It must be unique so FindWindow can locate the HWND Wails does not expose.
const mapWindowClass = "TarkovNexus.MapWindow"

// MapWindowApp is a minimal Wails app that displays tarkov.dev with party markers injected.
// It has no integration services — it is a pure viewer window.
type MapWindowApp struct {
	ctx      context.Context
	remoteID string
	mapName  string

	// Window placement. store is guarded by mu because the poller goroutine
	// and the pin toggle (UI thread) both save. pinMu serialises the whole
	// toggle (OS call included) so two rapid clicks cannot both sample the
	// same prior value.
	mu            sync.Mutex
	pinMu         sync.Mutex
	resizeMu      sync.Mutex
	store         *mapwindow.Store
	state         mapwindow.State
	poller        *mapwindow.Poller
	hwnd          mapwindow.HWND
	appearanceSeq int
}

// ToggleAlwaysOnTop toggles the window's always-on-top state, persists it, and
// returns the new state.
func (m *MapWindowApp) ToggleAlwaysOnTop() bool {
	m.pinMu.Lock()
	defer m.pinMu.Unlock()

	m.mu.Lock()
	next := !m.state.Pinned
	m.mu.Unlock()

	wailsRuntime.WindowSetAlwaysOnTop(m.ctx, next)

	m.mu.Lock()
	m.state.Pinned = next
	m.saveLocked()
	m.mu.Unlock()
	return next
}

// GetPinned reports the restored pin state so the control pill can reflect it
// on load.
func (m *MapWindowApp) GetPinned() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Pinned
}

// GetResizeBounds snapshots physical bounds once at the start of an edge drag.
func (m *MapWindowApp) GetResizeBounds() (mapwindow.Rect, error) {
	m.resizeMu.Lock()
	defer m.resizeMu.Unlock()
	return mapwindow.WindowRect(m.hwnd)
}

// ResizeCircle accepts an absolute scale from the drag's original bounds.
// The browser cannot select a native window or bypass the DPI-aware minimum.
func (m *MapWindowApp) ResizeCircle(start mapwindow.Rect, scale float64) error {
	m.resizeMu.Lock()
	defer m.resizeMu.Unlock()
	return mapwindow.ResizeCircle(m.hwnd, start, scale)
}

// Appearance is the control pill's view of shape and map opacity. Wails
// marshals it as {"shape": "circle", "opacity": 60}.
type Appearance struct {
	Shape    string `json:"shape"`
	Opacity  int    `json:"opacity"`
	Sequence int    `json:"sequence"`
}

func (m *MapWindowApp) appearanceLocked() Appearance {
	return Appearance{Shape: string(m.state.EffectiveShape()), Opacity: m.state.EffectiveOpacity(), Sequence: m.appearanceSeq}
}

// GetAppearance returns the effective (defaulted, clamped) values for the
// pill to apply on load.
func (m *MapWindowApp) GetAppearance() Appearance {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appearanceLocked()
}

// SetAppearance stores both values, normalises them, saves, and returns what
// was actually stored so the pill can correct itself. clientSeq is the pill's
// persist counter: a slower older call must not overwrite a newer save.
// Save failures return an error so the pill can snap back.
func (m *MapWindowApp) SetAppearance(a Appearance, clientSeq int) (Appearance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if clientSeq < m.appearanceSeq {
		return m.appearanceLocked(), nil
	}
	next := m.state
	next.Shape = mapwindow.Shape(a.Shape)
	next.Opacity = a.Opacity
	next.Normalize()
	if err := m.store.Save(next); err != nil {
		return Appearance{}, err
	}
	m.appearanceSeq = clientSeq
	m.state = next
	return m.appearanceLocked(), nil
}

func (m *MapWindowApp) saveLocked() {
	if err := m.store.Save(m.state); err != nil {
		fmt.Println("map window: could not save state:", err)
	}
}

func (m *MapWindowApp) savePlacement(r mapwindow.Rect) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Placement = &r
	return m.store.Save(m.state)
}

func (m *MapWindowApp) startup(ctx context.Context) {
	m.ctx = ctx
	hwnd, target, placed := m.restorePlacement()
	m.resizeMu.Lock()
	m.hwnd = hwnd
	m.resizeMu.Unlock()
	// Always show, whatever happened above: a mis-placed window beats an
	// invisible one.
	wailsRuntime.WindowShow(ctx)
	if hwnd == 0 {
		return
	}
	if placed {
		// Defensive: if showing delivers a WM_DPICHANGED, Wails applies
		// Windows' suggested rect. Re-assert once on the final monitor.
		if err := mapwindow.PlaceExactly(hwnd, target); err != nil {
			fmt.Println("map window: re-place after show failed:", err)
		}
	}
	m.startPoller(hwnd)
}

// restorePlacement moves the (still hidden) window to its remembered rect.
// Every native failure is logged and degraded, never fatal:
//   - no HWND            → nothing placed, nothing polled (hwnd == 0)
//   - monitors unreadable → saved rect used unclamped
//   - SetWindowPos fails  → window stays where Wails put it
//
// placed reports whether target is actually on screen.
func (m *MapWindowApp) restorePlacement() (hwnd mapwindow.HWND, target mapwindow.Rect, placed bool) {
	hwnd, err := mapwindow.FindWindow(mapWindowClass)
	if err != nil {
		fmt.Println("map window: placement skipped:", err)
		return 0, mapwindow.Rect{}, false
	}

	m.mu.Lock()
	saved := m.state.Placement
	m.mu.Unlock()

	monitors, primary, err := mapwindow.Monitors()
	if err != nil {
		fmt.Println("map window: monitors unavailable, restoring unclamped:", err)
		if saved == nil {
			return hwnd, mapwindow.Rect{}, false
		}
		target = *saved
	} else {
		target = mapwindow.Clamp(saved, monitors, primary)
	}

	if err := mapwindow.PlaceExactly(hwnd, target); err != nil {
		fmt.Println("map window: placement skipped:", err)
		return hwnd, target, false
	}
	return hwnd, target, true
}

func (m *MapWindowApp) startPoller(hwnd mapwindow.HWND) {
	m.poller = mapwindow.NewPoller(
		func() (mapwindow.Rect, bool, error) {
			r, err := mapwindow.WindowRect(hwnd)
			if err != nil {
				return mapwindow.Rect{}, false, err
			}
			minimised, err := mapwindow.IsMinimised(hwnd)
			if err != nil {
				return mapwindow.Rect{}, false, err
			}
			return r, minimised, nil
		},
		m.savePlacement,
		mapwindow.PollerOptions{},
	)
	m.poller.Start()
}

// beforeClose flushes a pending placement write, then snapshots the live
// rect so a move that has not yet been sampled (up to one poll interval) is
// not lost. Only the pill's close button and the OS reach this path; the
// main app's shutdown Kills the child outright, which is why the poller
// saves continuously rather than only here.
func (m *MapWindowApp) beforeClose(ctx context.Context) bool {
	m.flushPlacement()
	return false
}

func (m *MapWindowApp) shutdown(ctx context.Context) {
	m.flushPlacement()
}

func (m *MapWindowApp) flushPlacement() {
	if m.poller != nil {
		m.poller.Stop()
	}
	if m.hwnd == 0 {
		return
	}
	minimised, err := mapwindow.IsMinimised(m.hwnd)
	if err != nil || minimised {
		return
	}
	r, err := mapwindow.WindowRect(m.hwnd)
	if err != nil || r.IsZero() || r.IsIconic() {
		return
	}
	if err := m.savePlacement(r); err != nil {
		fmt.Println("map window: could not save state:", err)
	}
}

// mapWindowStatePath is %APPDATA%\TarkovNexus\mapwindow.json. Kept apart from
// config.json: two processes writing one file is a corruption risk, and the
// map window child has no config plumbing.
func mapWindowStatePath() string {
	appData, err := os.UserConfigDir()
	if err != nil {
		return "mapwindow.json"
	}
	return filepath.Join(appData, "TarkovNexus", "mapwindow.json")
}

func (m *MapWindowApp) domReady(ctx context.Context) {
	mapPath := m.mapName
	if mapPath == "" {
		mapPath = "customs"
	}

	// Load tarkov.dev through our local reverse proxy.
	// The proxy strips X-Frame-Options/CSP and injects the party-markers script,
	// so the iframe can display tarkov.dev without Tampermonkey.
	// If we have a Remote ID, pass ?connection= so tarkov.dev auto-connects.
	// The injected script captures the active session ID and reports it back.
	proxyURL := fmt.Sprintf("http://localhost:44444/map/%s", mapPath)
	if m.remoteID != "" {
		proxyURL += fmt.Sprintf("?connection=%s", m.remoteID)
	}

	js := fmt.Sprintf(`document.getElementById('map-frame').src = %q;`, proxyURL)
	wailsRuntime.WindowExecJS(ctx, js)
}

// RunMapWindow launches the standalone map viewer window.
func RunMapWindow(remoteID, mapName string) {
	app := &MapWindowApp{
		remoteID: remoteID,
		mapName:  mapName,
		store:    mapwindow.NewStore(mapWindowStatePath()),
	}

	var ok bool
	if app.state, ok = app.store.Load(); !ok {
		fmt.Println("map window: no saved placement, using defaults")
	}

	// Strip the subdirectory prefix so index.html is at the FS root
	sub, err := fs.Sub(mapWindowAssets, "mapwindow_assets")
	if err != nil {
		fmt.Println("map window: failed to load assets:", err)
		return
	}

	// Persistent WebView2 data directory so tarkov.dev cookies, localStorage,
	// and map settings survive between sessions.
	var webviewUserDataPath string
	if appData, err := os.UserConfigDir(); err == nil {
		webviewUserDataPath = filepath.Join(appData, "TarkovNexus", "MapWindow")
	}

	size := mapwindow.LaunchSize(app.state)
	wails.Run(&options.App{ //nolint:errcheck
		Title:     "Party Map – Tarkov Nexus",
		Width:     size.W,
		Height:    size.H,
		MinWidth:  200,
		MinHeight: 200,
		AssetServer: &assetserver.Options{
			Assets: sub,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		Frameless:        true, // control pill (pin / shape / opacity / minimise / close)
		StartHidden:      true, // shown by startup once placed, so there is no jump
		AlwaysOnTop:      app.state.Pinned,
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:                             windows.Dark,
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               true,
			WebviewUserDataPath:               webviewUserDataPath,
			DisableFramelessWindowDecorations: true, // DWM's 1px frame draws a rectangle around a circle map
			WindowClassName:                   mapWindowClass,
		},
	})
}
