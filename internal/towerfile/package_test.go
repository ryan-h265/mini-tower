package towerfile

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// readArchiveEntries extracts all entry names from a tar.gz reader.
func readArchiveEntries(t *testing.T, r io.Reader) []string {
	t.Helper()
	gr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		names = append(names, hdr.Name)
	}
	sort.Strings(names)
	return names
}

func TestPackageBasic(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/util.py",
		"requirements.txt",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./**/*.py", "./requirements.txt", "./Towerfile"},
		},
	}

	r, sha, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}
	if sha == "" {
		t.Fatal("SHA256 should not be empty")
	}

	entries := readArchiveEntries(t, r)
	want := []string{"Towerfile", "lib/util.py", "main.py", "requirements.txt"}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entries[%d] = %q, want %q", i, entries[i], want[i])
		}
	}
}

func TestPackageSHA256Correct(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.py", "./Towerfile"},
		},
	}

	r, reportedSHA, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	// Read all bytes and compute SHA256 independently.
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	h := sha256.Sum256(data)
	computedSHA := hex.EncodeToString(h[:])

	if reportedSHA != computedSHA {
		t.Errorf("SHA256 mismatch: reported %s, computed %s", reportedSHA, computedSHA)
	}
}

func TestPackageTowerfileAlwaysIncluded(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	// Source patterns don't include Towerfile explicitly.
	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.py"},
		},
	}

	r, _, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	entries := readArchiveEntries(t, r)
	found := false
	for _, e := range entries {
		if e == "Towerfile" {
			found = true
		}
	}
	if !found {
		t.Errorf("Towerfile should always be included, entries: %v", entries)
	}
}

func TestPackageTowerfileNotDuplicated(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	// Source patterns explicitly include Towerfile.
	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.py", "./Towerfile"},
		},
	}

	r, _, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	entries := readArchiveEntries(t, r)
	count := 0
	for _, e := range entries {
		if e == "Towerfile" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Towerfile appeared %d times, want 1", count)
	}
}

func TestPackageScriptNotInSource(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"other.txt",
		"Towerfile",
	})

	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.txt"}, // doesn't match main.py
		},
	}

	_, _, err := Package(dir, tf)
	if err == nil {
		t.Fatal("Package() should fail when script is not in source")
	}
}

func TestPackageValidationFailure(t *testing.T) {
	dir := setupTestDir(t, []string{"main.py"})

	// Missing name → validation error.
	tf := &Towerfile{App: App{Script: "main.py"}}

	_, _, err := Package(dir, tf)
	if err == nil {
		t.Fatal("Package() should fail on validation error")
	}
}

func TestPackageDefaultSourceGlob(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/helper.py",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	// No source patterns → defaults to ["./**"].
	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
		},
	}

	r, _, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	entries := readArchiveEntries(t, r)
	if len(entries) < 2 {
		t.Errorf("expected at least 2 files, got %v", entries)
	}
}

func TestPackageFileContent(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"Towerfile",
	})
	content := []byte("print('hello world')")
	os.WriteFile(filepath.Join(dir, "main.py"), content, 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.py", "./Towerfile"},
		},
	}

	r, _, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	// Verify file content is preserved in the archive.
	data, _ := io.ReadAll(r)
	gr, _ := gzip.NewReader(bytes.NewReader(data))
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Name == "main.py" {
			got, _ := io.ReadAll(tr)
			if !bytes.Equal(got, content) {
				t.Errorf("main.py content = %q, want %q", got, content)
			}
			return
		}
	}
	t.Fatal("main.py not found in archive")
}

func TestPackageSymlinkEntry(t *testing.T) {
	dir := setupTestDir(t, []string{
		"main.py",
		"lib/util.py",
		"Towerfile",
	})
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("import lib.util"), 0o644)
	os.WriteFile(filepath.Join(dir, "Towerfile"), []byte("[app]\nname=\"test-app\"\nscript=\"main.py\""), 0o644)

	// Create a symlink inside the project.
	symlink := filepath.Join(dir, "link.py")
	if err := os.Symlink(filepath.Join(dir, "lib/util.py"), symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	tf := &Towerfile{
		App: App{
			Name:   "test-app",
			Script: "main.py",
			Source: []string{"./*.py", "./lib/*.py", "./Towerfile"},
		},
	}

	r, _, err := Package(dir, tf)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	// Verify the symlink is stored as TypeSymlink in the archive.
	data, _ := io.ReadAll(r)
	gr, _ := gzip.NewReader(bytes.NewReader(data))
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Name == "link.py" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Errorf("link.py should be TypeSymlink, got %d", hdr.Typeflag)
			}
			return
		}
	}
	t.Fatal("link.py not found in archive")
}
