package store

import "database/sql"

// Store wraps database operations.
type Store struct {
  db *sql.DB
}

// New creates a new Store.
func New(db *sql.DB) *Store {
  return &Store{db: db}
}
