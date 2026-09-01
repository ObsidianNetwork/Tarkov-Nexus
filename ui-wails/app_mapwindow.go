package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wailsapp/wails/v2"
)

//go:embed mapwindow_assets
var mapWindowAssets embed.FS

// MapWindowApp is a minimal Wails app that displays tarkov.dev with party markers injected.
// It has no integration services — it is a pure viewer window.
type MapWindowApp struct {
	ctx         context.Context
	remoteID    string
	mapName     string
	alwaysOnTop bool
}

// ToggleAlwaysOnTop toggles the window's always-on-top state and returns the new state.
func (m *MapWindowApp) ToggleAlwaysOnTop() bool {
	m.alwaysOnTop = !m.alwaysOnTop
	wailsRuntime.WindowSetAlwaysOnTop(m.ctx, m.alwaysOnTop)
	return m.alwaysOnTop
}

func (m *MapWindowApp) startup(ctx context.Context) {
	m.ctx = ctx
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

	wails.Run(&options.App{ //nolint:errcheck
		Title:     "Party Map – Tarkov Nexus",
		Width:     500,
		Height:    500,
		MinWidth:  200,
		MinHeight: 200,
		AssetServer: &assetserver.Options{
			Assets: sub,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		Frameless:        true, // custom title bar with pin-to-top button
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:                             windows.Dark,
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               true,
			WebviewUserDataPath:               webviewUserDataPath,
			DisableFramelessWindowDecorations: false,
		},
	})
}

