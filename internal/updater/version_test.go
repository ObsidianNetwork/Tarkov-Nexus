package updater

import (
	"testing"
)

func TestVersion_DevelopmentDefault(t *testing.T) {
	if Version != "0.0.0-dev" {
		t.Fatalf("unversioned build = %q, want development version", Version)
	}
	if _, err := normalizeSemver(Version); err != nil {
		t.Fatalf("development version must support update comparisons: %v", err)
	}
}

func TestCompareVersions_PrereleaseOrdering(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int // expected CompareVersions(v1, v2)
	}{
		{"beta sorts before its own release", "3.4.0-beta.1", "3.4.0", -1},
		{"later beta is newer", "3.4.0-beta.2", "3.4.0-beta.1", 1},
		{"next beta cycle beats old stable", "3.5.0-beta.1", "3.4.0", 1},
		{"stable equal", "3.4.0", "3.4.0", 0},
		{"patch bump", "3.4.1", "3.4.0", 1},
		{"minor bump", "3.5.0", "3.4.9", 1},
		{"v prefix ignored", "v3.4.0", "3.4.0", 0},
		{"two-part pads to three", "3.4", "3.4.0", 0},
		{"rc sorts before release", "3.4.0-rc.1", "3.4.0", -1},
		{"alpha before beta", "3.4.0-alpha.1", "3.4.0-beta.1", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.v1, tt.v2)
			if err != nil {
				t.Fatalf("CompareVersions(%q, %q) error: %v", tt.v1, tt.v2, err)
			}
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestCompareVersions_Invalid(t *testing.T) {
	for _, v := range []string{"", "abc", "3.4.0.1", "3.x.0"} {
		if _, err := CompareVersions(v, "3.4.0"); err == nil {
			t.Errorf("CompareVersions(%q, ...) expected error, got nil", v)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	// 3.3.3 fleet compatibility: three-part candidate versions must compare.
	newer, err := IsNewerVersion("3.3.3", "3.4.0")
	if err != nil || !newer {
		t.Errorf("3.4.0 should be newer than 3.3.3 (newer=%v, err=%v)", newer, err)
	}
	newer, err = IsNewerVersion("3.3.3", "3.3.3")
	if err != nil || newer {
		t.Errorf("3.3.3 should not be newer than itself (newer=%v, err=%v)", newer, err)
	}
	// Prerelease awareness for the new updater.
	newer, err = IsNewerVersion("3.4.0-beta.1", "3.4.0")
	if err != nil || !newer {
		t.Errorf("stable 3.4.0 should be newer than its beta (newer=%v, err=%v)", newer, err)
	}
	newer, err = IsNewerVersion("3.4.0", "3.4.0-beta.2")
	if err != nil || newer {
		t.Errorf("beta must not be newer than its release (newer=%v, err=%v)", newer, err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in                  string
		major, minor, patch int
	}{
		{"3.3.3", 3, 3, 3},
		{"v3.4.0", 3, 4, 0},
		{"3.4", 3, 4, 0},
		{"3.4.0-beta.1", 3, 4, 0},
	}
	for _, tt := range tests {
		major, minor, patch, err := ParseVersion(tt.in)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", tt.in, err)
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d", tt.in, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
	}
	if _, _, _, err := ParseVersion("not-a-version"); err == nil {
		t.Error("ParseVersion(not-a-version) expected error")
	}
}
