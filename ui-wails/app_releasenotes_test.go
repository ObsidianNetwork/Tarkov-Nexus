package main

import (
	"context"
	"os"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/internal/updater"
)

func TestReleaseNotes_RequiresApplicationContext(t *testing.T) {
	app := NewApp(t.TempDir())
	_, err := app.RefreshReleaseNotes("3.3.3")
	if err == nil {
		t.Fatal("binding accepted an unavailable application context")
	}
}

func TestReleaseNotes_DelegatesWithoutSubstitutingDevelopmentVersion(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(dir)
	app.ctx = context.Background()
	version := "development"

	result, err := app.RefreshReleaseNotes(version)

	if err != nil {
		t.Fatal(err)
	}
	if result.Version != version || result.Tag != "" {
		t.Fatalf("requested identity changed: %+v", result)
	}
	if result.Lookup != "missing" || result.Saved || result.Note != nil {
		t.Fatalf("development version substituted with release content: %+v", result)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fresh-session reader wrote application data: %v", entries)
	}
}

func TestReleaseNotes_OfferedPayloadIsAvailableAfterRestart(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(dir)
	app.rememberOfferedRelease(&updater.UpdateInfo{
		Version: "3.3.4", ReleaseURL: "https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/tag/v3.3.4",
		ReleaseDate: time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC), ReleaseName: "Release", ReleaseBody: "# Offered notes",
	})

	restarted := NewApp(dir)
	result, err := restarted.GetSavedReleaseNotes("3.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if result.Note == nil || result.Note.Body != "# Offered notes" || !result.Saved || result.Lookup != updater.NoteUnchecked {
		t.Fatalf("offered payload did not survive restart: %+v", result)
	}
}

func TestReleaseNotesLink_RejectsUnsafeDestinations(t *testing.T) {
	for _, link := range []string{"https://github.com/owner/repo/releases/tag/v1.2.3", "http://example.com/help?q=notes#section"} {
		if got, err := releaseNotesLink(link); err != nil || got != link {
			t.Errorf("safe link rejected: %q %v", got, err)
		}
	}
	for _, link := range []string{"javascript:alert(1)", "data:text/html,test", "file:///C:/test", "blob:https://example.com/test", "//example.com", "/notes", "https://user:pass@example.com", "https://", "https://example.com\n", "https://example.com\\evil"} {
		if _, err := releaseNotesLink(link); err == nil {
			t.Errorf("unsafe link accepted: %q", link)
		}
		if err := NewApp(t.TempDir()).OpenReleaseNotesLink(link); err == nil {
			t.Errorf("unsafe link reached browser adapter: %q", link)
		}
	}
}
