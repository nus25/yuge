package subscriber

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type jetstreamCursorStore struct {
	path string
}

func newJetstreamCursorStore(dataDir string) *jetstreamCursorStore {
	return &jetstreamCursorStore{path: filepath.Join(dataDir, "jetstream-cursor.json")}
}

func (store *jetstreamCursorStore) Load() (int64, error) {
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return -1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read jetstream cursor: %w", err)
	}
	var value struct {
		Cursor int64 `json:"cursor"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("decode jetstream cursor: %w", err)
	}
	if value.Cursor <= 0 {
		return 0, fmt.Errorf("invalid jetstream cursor: %d", value.Cursor)
	}
	return value.Cursor, nil
}

func (store *jetstreamCursorStore) Save(cursor int64) error {
	dir := filepath.Dir(store.path)
	if cursor <= 0 {
		if err := os.Remove(store.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove jetstream cursor: %w", err)
		}
	} else {
		data, err := json.Marshal(struct {
			Cursor int64 `json:"cursor"`
		}{Cursor: cursor})
		if err != nil {
			return fmt.Errorf("encode jetstream cursor: %w", err)
		}
		file, err := os.CreateTemp(dir, ".jetstream-cursor-*")
		if err != nil {
			return fmt.Errorf("create jetstream cursor file: %w", err)
		}
		defer os.Remove(file.Name())
		if _, err = file.Write(data); err == nil {
			err = file.Sync()
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("write jetstream cursor: %w", err)
		}
		if err := os.Rename(file.Name(), store.path); err != nil {
			return fmt.Errorf("replace jetstream cursor: %w", err)
		}
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open jetstream cursor directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync jetstream cursor directory: %w", err)
	}
	return nil
}

func initialJetstreamCursor(store *jetstreamCursorStore, override *int64) (int64, error) {
	if override != nil {
		if err := store.Save(*override); err != nil {
			return 0, err
		}
		return *override, nil
	}
	return store.Load()
}
