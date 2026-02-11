package objects

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStore stores objects on the local filesystem.
type LocalStore struct {
	dir string
}

// NewLocalStore creates a new local object store.
func NewLocalStore(dir string) (*LocalStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create objects dir: %w", err)
	}
	return &LocalStore{dir: dir}, nil
}

// Store writes an object from a reader.
func (s *LocalStore) Store(key string, r io.Reader) error {
	path := filepath.Join(s.dir, key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return f.Close()
}

// Load returns a reader for an object.
func (s *LocalStore) Load(key string) (io.ReadCloser, error) {
	path := filepath.Join(s.dir, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

// Delete removes an object if it exists.
func (s *LocalStore) Delete(key string) error {
	path := filepath.Join(s.dir, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

// Exists checks if an object exists.
func (s *LocalStore) Exists(key string) (bool, error) {
	path := filepath.Join(s.dir, key)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
