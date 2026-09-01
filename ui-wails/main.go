package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	mapWindow  := flag.Bool("map-window", false, "Open as a standalone party map window")
	remoteID   := flag.String("remote-id", "", "tarkov.dev Remote ID (used in map-window mode)")
	mapName    := flag.String("map-name", "customs", "Initial map to display (used in map-window mode)")
	flag.Parse()

	// If launched as a map window, run the lightweight viewer and exit
	if *mapWindow {
		RunMapWindow(*remoteID, *mapName)
		return
	}

	// Resolve data directory in AppData so no files are created next to the exe.
	dataDir := resolveDataDir(*configPath)

	// Normal application launch
	app := NewApp(dataDir)

	err := wails.Run(&options.App{
		Title:     "Tarkov Nexus",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 14, B: 20, A: 1},
		Frameless:        true,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:                             windows.Dark,
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               false,
			BackdropType:                      windows.Auto,
			DisableFramelessWindowDecorations: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// resolveDataDir returns the application data directory.
// If an explicit --config path was given, use its parent directory.
// Otherwise default to %AppData%/TarkovNexus, creating it if needed.
// If a config.json exists next to the exe (legacy location), migrate it.
func resolveDataDir(explicitConfig string) string {
	if explicitConfig != "" {
		return filepath.Dir(explicitConfig)
	}

	appData, err := os.UserConfigDir() // %AppData% on Windows
	if err != nil {
		// Fallback to CWD
		return ""
	}

	dir := filepath.Join(appData, "TarkovNexus")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: could not create data directory %s: %v\n", dir, err)
		return ""
	}

	// Migrate legacy config.json from CWD → AppData (one-time)
	legacyCfg := "config.json"
	newCfg := filepath.Join(dir, "config.json")
	if _, err := os.Stat(legacyCfg); err == nil {
		if _, err := os.Stat(newCfg); os.IsNotExist(err) {
			if data, err := os.ReadFile(legacyCfg); err == nil {
				if err := os.WriteFile(newCfg, data, 0644); err == nil {
					fmt.Printf("Migrated config.json → %s\n", newCfg)
					os.Remove(legacyCfg) // Clean up old file
				}
			}
		}
	}

	return dir
}
