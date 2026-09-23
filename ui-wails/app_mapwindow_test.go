package main

import (
	"path/filepath"
	"testing"

	"tarkov-screenshot-analyzer/internal/mapwindow"
)

func TestMapWindow_AppearanceSnapshotResumesAfterReload(t *testing.T) {
	app := &MapWindowApp{
		store: mapwindow.NewStore(filepath.Join(t.TempDir(), "mapwindow.json")),
		state: mapwindow.Defaults(),
	}
	if _, err := app.SetAppearance(Appearance{Shape: "circle", Opacity: 60}, 42); err != nil {
		t.Fatal(err)
	}
	restored := app.GetAppearance()
	got, err := app.SetAppearance(Appearance{Shape: "square", Opacity: 80}, restored.Sequence+1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shape != "square" || got.Opacity != 80 || got.Sequence != 43 {
		t.Fatalf("appearance after reloading controls = %+v", got)
	}
	saved, ok := app.store.Load()
	if !ok || saved.Shape != mapwindow.ShapeSquare || saved.Opacity != 80 {
		t.Fatalf("persisted appearance after reload = %+v, loaded=%t", saved, ok)
	}
}
