package mapwindow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Store persists whole mapwindow states. Independent stores may share a path;
// their last successful replacement wins without merging individual fields.
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

// Save writes and syncs a unique sibling temporary file before replacing state.
// It creates the directory if needed. Windows does not guarantee an atomic
// rename or crash durability; Load falls back to defaults for unreadable state.
func (s *Store) Save(st State) (err error) {
	st.V = SchemaVersion
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if cleanupErr := os.Remove(tmp); !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err := writeSynced(f, data); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// writeSynced owns f, writes and syncs its contents, and always closes it.
func writeSynced(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}
