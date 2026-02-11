package towerfile

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDir creates a temp directory with the given file structure.
// Files are created with empty content. Returns the temp dir path.
func setupTestDir(t *testing.T, files []string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveSourceBasic(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/util.py",
		"lib/helper.py",
		"requirements.txt",
		"README.md",
	})

	files, err := ResolveSource(dir, []string{"./**/*.py", "./requirements.txt"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	want := []string{"lib/helper.py", "lib/util.py", "main.py", "requirements.txt"}
	if len(files) != len(want) {
		t.Fatalf("got %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, files[i], want[i])
		}
	}
}

func TestResolveSourceAllFiles(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"sub/a.txt",
	})

	files, err := ResolveSource(dir, []string{"./**"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	// Should include both files but not directories.
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(files), files)
	}
}

func TestResolveSourceDedup(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/util.py",
	})

	// Both patterns match main.py.
	files, err := ResolveSource(dir, []string{"./**/*.py", "./main.py"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	count := 0
	for _, f := range files {
		if f == "main.py" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("main.py appeared %d times, want 1", count)
	}
}

func TestResolveSourceSorted(t *testing.T) {
	dir := setupTestDir(t, []string{
		"c.py",
		"a.py",
		"b.py",
	})

	files, err := ResolveSource(dir, []string{"./*.py"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Fatalf("files not sorted: %v", files)
		}
	}
}

func TestResolveSourceDotfilesExcluded(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		".env",
		".secret/key",
	})

	files, err := ResolveSource(dir, []string{"./**"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	for _, f := range files {
		if f == ".env" || f == ".secret/key" {
			t.Errorf("dotfile %q should not be matched by **", f)
		}
	}
}

func TestResolveSourceDotfilesExplicit(t *testing.T) {
	dir := setupTestDir(t, []string{
		".env",
	})

	files, err := ResolveSource(dir, []string{"./.env"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	if len(files) != 1 || files[0] != ".env" {
		t.Errorf("got %v, want [.env]", files)
	}
}

func TestResolveSourceTraversalRejected(t *testing.T) {
	dir := setupTestDir(t, []string{"main.py"})

	_, err := ResolveSource(dir, []string{"../../etc/passwd"})
	if err == nil {
		t.Fatal("ResolveSource() should reject path traversal")
	}
}

func TestResolveSourceNoMatches(t *testing.T) {
	dir := setupTestDir(t, []string{"main.py"})

	files, err := ResolveSource(dir, []string{"./**/*.rb"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected no matches, got %v", files)
	}
}

func TestResolveSourceSymlinkInside(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/util.py",
	})

	// Create a symlink inside the project root.
	symlink := filepath.Join(dir, "link.py")
	target := filepath.Join(dir, "lib/util.py")
	if err := os.Symlink(target, symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	files, err := ResolveSource(dir, []string{"./*.py"})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}

	found := false
	for _, f := range files {
		if f == "link.py" {
			found = true
		}
	}
	if !found {
		t.Errorf("symlink link.py should be in results: %v", files)
	}
}

func TestResolveSourceSymlinkOutside(t *testing.T) {
	dir := setupTestDir(t, []string{"main.py"})

	// Create a temp file outside the project and symlink to it.
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.py")
	if err := os.WriteFile(outsideFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	symlink := filepath.Join(dir, "escape.py")
	if err := os.Symlink(outsideFile, symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err := ResolveSource(dir, []string{"./*.py"})
	if err == nil {
		t.Fatal("ResolveSource() should reject symlinks pointing outside the project")
	}
}

func TestResolveSourceEmptyPatterns(t *testing.T) {
	dir := setupTestDir(t, []string{"main.py"})

	files, err := ResolveSource(dir, []string{})
	if err != nil {
		t.Fatalf("ResolveSource() error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no files for empty patterns, got %v", files)
	}
}
