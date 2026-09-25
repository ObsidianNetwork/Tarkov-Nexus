package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"tarkov-screenshot-analyzer/internal/updater"
)

func (a *App) RefreshReleaseNotes(version string) (updater.ReleaseNotesResult, error) {
	if a.ctx == nil || a.releaseNotes == nil {
		return updater.ReleaseNotesResult{}, errors.New("release notes are not initialized")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	result := a.releaseNotes.Refresh(ctx, version)
	if result.Lookup == updater.NoteFailed {
		a.logWarning("Release notes refresh failed")
	}
	return result, nil
}

func (a *App) GetSavedReleaseNotes(version string) (updater.ReleaseNotesResult, error) {
	if a.releaseNotes == nil {
		return updater.ReleaseNotesResult{}, errors.New("release notes are not initialized")
	}
	return a.releaseNotes.Read(version), nil
}

func (a *App) rememberOfferedRelease(info *updater.UpdateInfo) {
	if a.releaseNotes == nil || info == nil {
		return
	}
	result := a.releaseNotes.RememberOffer(*info)
	if result.Note != nil && !result.Saved {
		a.logWarning("Offered release notes are available for this session but could not be saved")
	}
}

func releaseNotesLink(rawURL string) (string, error) {
	if strings.ContainsAny(rawURL, "\\") || strings.IndexFunc(rawURL, unicode.IsControl) >= 0 {
		return "", errors.New("invalid release notes link")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", errors.New("release notes links must be HTTP or HTTPS without credentials")
	}
	return parsed.String(), nil
}

func (a *App) OpenReleaseNotesLink(rawURL string) error {
	link, err := releaseNotesLink(rawURL)
	if err != nil {
		return err
	}
	if a.ctx == nil {
		return errors.New("application context is unavailable")
	}
	wailsRuntime.BrowserOpenURL(a.ctx, link)
	return nil
}
