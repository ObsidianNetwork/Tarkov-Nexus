package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the current application version.
// Set at build time via: go build -ldflags "-X tarkov-screenshot-analyzer/internal/updater.Version=3.3.0"
// Falls back to this default if not overridden.
var Version = "3.3.3"

// ParseVersion parses a semantic version string (e.g., "v3.0.0" or "3.0.0")
// Returns major, minor, patch as integers
func ParseVersion(version string) (major, minor, patch int, err error) {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", version)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %w", err)
	}

	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %w", err)
	}

	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %w", err)
	}

	return major, minor, patch, nil
}

// CompareVersions compares two semantic version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if v1 == v2
func CompareVersions(v1, v2 string) (int, error) {
	major1, minor1, patch1, err := ParseVersion(v1)
	if err != nil {
		return 0, fmt.Errorf("invalid version v1: %w", err)
	}

	major2, minor2, patch2, err := ParseVersion(v2)
	if err != nil {
		return 0, fmt.Errorf("invalid version v2: %w", err)
	}

	if major1 > major2 {
		return 1, nil
	} else if major1 < major2 {
		return -1, nil
	}

	if minor1 > minor2 {
		return 1, nil
	} else if minor1 < minor2 {
		return -1, nil
	}

	if patch1 > patch2 {
		return 1, nil
	} else if patch1 < patch2 {
		return -1, nil
	}

	return 0, nil
}

// IsNewerVersion returns true if newVersion is newer than currentVersion
func IsNewerVersion(currentVersion, newVersion string) (bool, error) {
	result, err := CompareVersions(newVersion, currentVersion)
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// FormatVersion ensures version has 'v' prefix
func FormatVersion(version string) string {
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}
