//go:build !windows

package autoscreenshot

func NewPlatformDriver() Driver { return nil }
