package updater

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Version is the current application version.
// Overridden at build time by release.yml via:
//
//	go build -ldflags "-X tarkov-screenshot-analyzer/internal/updater.Version=3.4.0"
//
// Local builds use a semver-compatible sentinel, displayed as DEV in the sidebar.
var Version = "0.0.0-dev"

// normalizeSemver converts a version string into the canonical form the
// semver package requires: a "v" prefix and exactly three numeric parts.
// Accepts "3.4", "v3.4", "3.4.0", "3.4.0-beta.1" and pads missing parts
// with zeros ("3.4" becomes "v3.4.0"). Prerelease/build suffixes are kept,
// so "v3.4.0-beta.1" orders before "v3.4.0" per semver rules.
func normalizeSemver(version string) (string, error) {
	v := strings.TrimSpace(version)
	if v == "" {
		return "", fmt.Errorf("empty version")
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}

	core, suffix := v[1:], ""
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		suffix = core[i:]
		core = core[:i]
	}

	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return "", fmt.Errorf("invalid version format: %s", version)
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return "", fmt.Errorf("invalid version component in %s: %w", version, err)
		}
	}

	canonical := "v" + strings.Join(parts, ".") + suffix
	if !semver.IsValid(canonical) {
		return "", fmt.Errorf("invalid semver: %s", version)
	}
	return canonical, nil
}

// ParseVersion parses a version string (e.g. "v3.4.0", "3.4", "3.4.0-beta.1")
// and returns the base major, minor and patch numbers. Prerelease suffixes
// are ignored; missing parts are zero.
func ParseVersion(version string) (major, minor, patch int, err error) {
	canonical, err := normalizeSemver(version)
	if err != nil {
		return 0, 0, 0, err
	}
	base := strings.TrimSuffix(strings.TrimPrefix(canonical, "v"), prereleaseSuffix(canonical))
	parts := strings.Split(base, ".")
	major, _ = strconv.Atoi(parts[0])
	minor, _ = strconv.Atoi(parts[1])
	patch, _ = strconv.Atoi(parts[2])
	return major, minor, patch, nil
}

// prereleaseSuffix returns the prerelease/build suffix of a canonical semver
// string, or "" if it has none.
func prereleaseSuffix(canonical string) string {
	if i := strings.IndexAny(canonical, "-+"); i >= 0 {
		return canonical[i:]
	}
	return ""
}

// CompareVersions compares two version strings.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
// Prerelease-aware: "3.4.0-beta.1" < "3.4.0", "3.5.0-beta.1" > "3.4.0".
func CompareVersions(v1, v2 string) (int, error) {
	a, err := normalizeSemver(v1)
	if err != nil {
		return 0, fmt.Errorf("invalid version v1: %w", err)
	}
	b, err := normalizeSemver(v2)
	if err != nil {
		return 0, fmt.Errorf("invalid version v2: %w", err)
	}
	return semver.Compare(a, b), nil
}

// IsNewerVersion returns true if newVersion is newer than currentVersion.
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
