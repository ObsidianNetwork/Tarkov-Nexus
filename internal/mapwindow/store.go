package mapwindow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Store owns mapwindow.json. It is not safe for concurrent use; callers
// serialise (the map window holds one mutex around Save).
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads the state file. It never fails from the caller's view: a missing,
// unreadable, unparsable or unknown-version file yields Defaults() and
// ok=false so the caller can log it. The next Save overwrites the bad file.
func (s *Store) Load() (st State, ok bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Defaults(), false
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return Defaults(), false
	}
	if st.V != SchemaVersion {
		return Defaults(), false
	}
	if st.Placement != nil && st.Placement.IsZero() {
		st.Placement = nil
	}
	st.Normalize()
	return st, true
}

// Save writes and syncs <path>.tmp before renaming it over the state file.
// It creates the directory if needed. Windows does not guarantee an atomic
// rename or crash durability; Load falls back to defaults for unreadable state.
func (s *Store) Save(st State) error {
	st.V = SchemaVersion
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := writeSynced(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

// writeSynced writes data to path and syncs the file before closing it.
func writeSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}
