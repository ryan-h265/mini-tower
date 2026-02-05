package migrate

import (
  "context"
  "database/sql"
  "errors"
  "fmt"
  "io/fs"
  "path/filepath"
  "sort"
  "strconv"
  "strings"
  "time"
)

type Migrator struct {
  fs fs.FS
}

func New(migrations fs.FS) *Migrator {
  return &Migrator{fs: migrations}
}

func (m *Migrator) Apply(ctx context.Context, db *sql.DB) error {
  if err := ensureSchemaMigrations(ctx, db); err != nil {
    return err
  }

  files, err := fs.Glob(m.fs, "*.up.sql")
  if err != nil {
    return fmt.Errorf("list migrations: %w", err)
  }

  sort.Strings(files)

  for _, name := range files {
    version, err := parseVersion(name)
    if err != nil {
      return err
    }

    applied, err := isApplied(ctx, db, version)
    if err != nil {
      return err
    }
    if applied {
      continue
    }

    contents, err := fs.ReadFile(m.fs, name)
    if err != nil {
      return fmt.Errorf("read migration %s: %w", name, err)
    }

    sqlText := strings.TrimSpace(string(contents))
    if sqlText == "" {
      return fmt.Errorf("migration %s is empty", name)
    }

    if err := applyOne(ctx, db, version, sqlText); err != nil {
      return fmt.Errorf("apply migration %s: %w", name, err)
    }
  }

  return nil
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
  const query = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
  );`

  if _, err := db.ExecContext(ctx, query); err != nil {
    return fmt.Errorf("create schema_migrations: %w", err)
  }

  return nil
}

func isApplied(ctx context.Context, db *sql.DB, version int64) (bool, error) {
  var value int
  err := db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ? LIMIT 1", version).Scan(&value)
  if errors.Is(err, sql.ErrNoRows) {
    return false, nil
  }
  if err != nil {
    return false, fmt.Errorf("check migration %d: %w", version, err)
  }

  return true, nil
}

func applyOne(ctx context.Context, db *sql.DB, version int64, sqlText string) error {
  tx, err := db.BeginTx(ctx, nil)
  if err != nil {
    return fmt.Errorf("begin migration %d: %w", version, err)
  }
  defer func() {
    _ = tx.Rollback()
  }()

  if _, err := tx.ExecContext(ctx, sqlText); err != nil {
    return fmt.Errorf("exec migration %d: %w", version, err)
  }

  if _, err := tx.ExecContext(
    ctx,
    "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)",
    version,
    time.Now().UnixMilli(),
  ); err != nil {
    return fmt.Errorf("record migration %d: %w", version, err)
  }

  if err := tx.Commit(); err != nil {
    return fmt.Errorf("commit migration %d: %w", version, err)
  }

  return nil
}

func parseVersion(path string) (int64, error) {
  base := filepath.Base(path)
  i := 0
  for i < len(base) && base[i] >= '0' && base[i] <= '9' {
    i++
  }
  if i == 0 {
    return 0, fmt.Errorf("migration %s missing numeric prefix", base)
  }

  version, err := strconv.ParseInt(base[:i], 10, 64)
  if err != nil {
    return 0, fmt.Errorf("migration %s invalid version: %w", base, err)
  }

  return version, nil
}
