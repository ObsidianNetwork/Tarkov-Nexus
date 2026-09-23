package mapwindow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_LoadMissingFileReturnsDefaults(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "mapwindow.json"))
	st, ok := s.Load()
	if ok {
		t.Fatal("ok should be false for a missing file")
	}
	if st != Defaults() {
		t.Fatalf("got %+v, want defaults %+v", st, Defaults())
	}
}

func TestStore_LoadCorruptJSONReturnsDefaultsNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapwindow.json")
	if err := os.WriteFile(path, []byte(`{"v":1,"placement":{"x":`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, ok := NewStore(path).Load()
	if ok || st != Defaults() {
		t.Fatalf("corrupt file: ok=%v st=%+v, want defaults and ok=false", ok, st)
	}
}

func TestStore_LoadUnknownVersionReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapwindow.json")
	if err := os.WriteFile(path, []byte(`{"v":99,"placement":{"x":1,"y":2,"w":3,"h":4},"pinned":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, ok := NewStore(path).Load()
	if ok || st != Defaults() {
		t.Fatalf("unknown version: ok=%v st=%+v, want defaults and ok=false", ok, st)
	}
}

func TestStore_SaveThenLoadRoundTrips(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "mapwindow.json"))
	want := State{V: SchemaVersion, Placement: &Rect{X: -2000, Y: 300, W: 520, H: 480}, Pinned: true, Shape: ShapeCircle, Opacity: 60}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Load()
	if !ok {
		t.Fatal("ok should be true after a save")
	}
	if got.V != want.V || got.Pinned != want.Pinned || got.Shape != want.Shape || got.Opacity != want.Opacity ||
		got.Placement == nil || *got.Placement != *want.Placement {
		t.Fatalf("round trip: got %+v (placement %+v), want %+v (placement %+v)", got, got.Placement, want, want.Placement)
	}
}

func TestStore_SaveLeavesUnownedScratchFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mapwindow.json")
	// A different process may still own the legacy scratch filename.
	if err := os.WriteFile(path+".tmp", []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Save(State{V: SchemaVersion, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path + ".tmp"); err != nil || string(got) != "garbage" {
		t.Fatalf("unowned scratch file changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "mapwindow.json" || entries[1].Name() != "mapwindow.json.tmp" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory should hold only state and the unowned scratch file, got %v", names)
	}
}

func TestStore_SaveCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "mapwindow.json")
	if err := NewStore(path).Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

// TestState_JSONShapeIsV1Contract pins the on-disk format. Appearance fields
// are additive with omitempty, so v stays 1 and old files still load.
func TestState_JSONShapeIsV1Contract(t *testing.T) {
	st := State{V: 1, Placement: &Rect{X: 2135, Y: 612, W: 520, H: 480}, Pinned: true, Shape: ShapeCircle, Opacity: 60}
	got, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"placement":{"x":2135,"y":612,"w":520,"h":480},"pinned":true,"shape":"circle","opacity":60}`
	if string(got) != want {
		t.Fatalf("json shape changed:\n got %s\nwant %s", got, want)
	}

	// Never saved: placement, shape and opacity are omitted entirely.
	got, _ = json.Marshal(Defaults())
	want = `{"v":1,"pinned":false}`
	if string(got) != want {
		t.Fatalf("defaults json shape changed:\n got %s\nwant %s", got, want)
	}
}

func TestStore_LoadNormalizesAppearance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapwindow.json")
	if err := os.WriteFile(path, []byte(`{"v":1,"pinned":false,"shape":"blob","opacity":7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, ok := NewStore(path).Load()
	if !ok {
		t.Fatal("bad appearance values are not a corrupt file; ok must be true")
	}
	if st.Shape != ShapeSquare || st.Opacity != OpacityMin {
		t.Fatalf("got shape=%q opacity=%d, want square/%d", st.Shape, st.Opacity, OpacityMin)
	}
}

func TestStore_LoadOldFileWithoutAppearanceGetsDefaults(t *testing.T) {
	// A file written by the placement plan, before shape/opacity existed.
	path := filepath.Join(t.TempDir(), "mapwindow.json")
	if err := os.WriteFile(path, []byte(`{"v":1,"placement":{"x":1,"y":2,"w":500,"h":500},"pinned":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, ok := NewStore(path).Load()
	if !ok || st.Shape != ShapeSquare || st.Opacity != OpacityDefault || !st.Pinned {
		t.Fatalf("old file: ok=%v %+v", ok, st)
	}
}
