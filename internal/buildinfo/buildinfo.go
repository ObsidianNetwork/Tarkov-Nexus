package buildinfo

import "os"

// Build-time variables set via -ldflags
// These replace Go's build tags to avoid triggering Fyne's debug mode
var BuildMode string = "release" // Set via -ldflags "-X buildinfo.BuildMode=debug"
var EnableConsole string = "false" // Set via -ldflags "-X buildinfo.EnableConsole=true"
var EnableDebugLogging string = "false" // Set via -ldflags "-X buildinfo.EnableDebugLogging=true"

func init() {
	// Aggressively disable Fyne debug mode regardless of build configuration
	os.Unsetenv("FYNE_DEBUG")
	os.Setenv("FYNE_DEBUG", "0")
	os.Setenv("FYNE_SCALE", "")
}

// HasConsole returns true if the application should show a console window
func HasConsole() bool {
	return EnableConsole == "true"
}

// IsDebugBuild returns true if this is a debug build
func IsDebugBuild() bool {
	return BuildMode == "debug"
}

// IsDebugLoggingEnabled returns true if debug logging should be enabled
func IsDebugLoggingEnabled() bool {
	return EnableDebugLogging == "true"
}