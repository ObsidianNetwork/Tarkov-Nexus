//go:build dev

package main

// OpenMapWindow — dev build.
//
// wails dev compiles with -tags dev and injects its hot-reload (HMR) JavaScript
// at the WebView level. That HMR script persists across navigation, so if we
// spawn a second Wails process and navigate it to tarkov.dev the HMR loop causes
// a constant page-refresh spasm.
//
// In dev mode we simply open tarkov.dev in the system browser with the correct
// Remote ID and current map.  The full native second window is available after
// `wails build`.
func (a *App) OpenMapWindow() error {
	a.logInfo("Dev mode: opening tarkov.dev in browser (native map window available after wails build)")
	return a.openTarkovDevMap()
}
