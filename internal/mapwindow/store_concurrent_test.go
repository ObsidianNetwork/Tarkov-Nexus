package mapwindow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_IndependentConcurrentWritersKeepCompleteStates(t *testing.T) {
	// Given independent stores sharing one profile, as separate app processes do.
	dir := t.TempDir()
	path := filepath.Join(dir, "mapwindow.json")
	const writers = 32
	start := make(chan struct{})
	type result struct {
		id  int
		err error
	}
	results := make(chan result, writers)
	for i := range writers {
		store := NewStore(path)
		state := State{V: SchemaVersion, Placement: &Rect{X: i, Y: i * 10, W: 500 + i, H: 600 + i}, Shape: ShapeCircle, Opacity: 60}
		go func() {
			<-start
			results <- result{id: i, err: store.Save(state)}
		}()
	}

	// When independent writes overlap on the same destination.
	close(start)
	succeeded := make(map[int]bool)
	for range writers {
		r := <-results
		if r.err == nil {
			succeeded[r.id] = true
			continue
		}
		var replacementErr *os.LinkError
		if !errors.As(r.err, &replacementErr) || replacementErr.Op != "rename" || replacementErr.New != path {
			t.Errorf("independent store failed before replacement: %v", r.err)
		}
	}
	if len(succeeded) == 0 {
		t.Fatal("no independent writer successfully replaced the state")
	}

	// Then one complete submitted state remains and no writer leaves scratch files.
	got, ok := NewStore(path).Load()
	if !ok || got.Placement == nil {
		t.Fatalf("concurrent writes left unreadable state: %+v, loaded=%v", got, ok)
	}
	r := got.Placement
	if !succeeded[r.X] || r.Y != r.X*10 || r.W != 500+r.X || r.H != 600+r.X || got.Shape != ShapeCircle || got.Opacity != 60 {
		t.Fatalf("concurrent writes mixed state fields: %+v, placement=%+v", got, r)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "mapwindow.json" {
		t.Fatalf("concurrent writers left unexpected directory entries: %v", entries)
	}
}

func TestStore_FailedReplacementRemovesOnlyOwnScratchFile(t *testing.T) {
	// Given a blocked destination and a scratch file owned by another writer.
	dir := t.TempDir()
	path := filepath.Join(dir, "mapwindow.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".tmp", []byte("other writer"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When replacement fails after the new state has been written and synced.
	err := NewStore(path).Save(Defaults())

	// Then the error is reported, this writer cleans up, and the other file survives.
	if err == nil {
		t.Fatal("replacing a directory must fail")
	}
	if got, err := os.ReadFile(path + ".tmp"); err != nil || string(got) != "other writer" {
		t.Fatalf("failed save changed another writer's scratch file: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "mapwindow.json" || entries[1].Name() != "mapwindow.json.tmp" {
		t.Fatalf("failed replacement left owned scratch files: %v", entries)
	}
}
