//go:build !dev

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OpenMapWindow — production build.
//
// Spawns a second instance of this executable with --map-window, which opens
// a standalone Tarkov Nexus window that loads tarkov.dev and auto-injects the
// party markers script.  No browser, no Tampermonkey.
func (a *App) OpenMapWindow() error {
	exe, err := os.Executable()
	if err != nil {
		a.logWarning("Could not resolve executable path, falling back to browser")
		return a.openTarkovDevMap()
	}

	args := []string{"--map-window"}

	if a.config.RemoteID != "" && a.config.RemoteID != "your_remote_id_here" {
		args = append(args, "--remote-id="+a.config.RemoteID)
	}

	// Open on the current detected map so the window shows the right map immediately
	status := a.GetStatus()
	if m, ok := status["currentNormalizedMap"].(string); ok && m != "" {
		args = append(args, "--map-name="+m)
	} else if m, ok := status["currentMap"].(string); ok && m != "" {
		args = append(args, "--map-name="+m)
	}

	cmd := exec.Command(exe, args...)

	// Strip dev-server env vars — the child is a production binary and must not
	// try to connect to a Vite dev server that isn't running.
	filtered := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WAILS_VITE_DEV_SERVER") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	if err := cmd.Start(); err != nil {
		a.logError(fmt.Sprintf("Failed to spawn map window: %v", err))
		return a.openTarkovDevMap()
	}

	a.logInfo("Opened party map window")
	return nil
}
