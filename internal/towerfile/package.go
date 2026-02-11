package towerfile

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Package validates the Towerfile, resolves source globs, packages the matched
// files plus the Towerfile itself into a tar.gz archive, and returns the
// archive bytes and hex-encoded SHA256.
func Package(dir string, tf *Towerfile) (io.Reader, string, error) {
	if err := Validate(tf); err != nil {
		return nil, "", fmt.Errorf("validation: %w", err)
	}

	patterns := tf.App.Source
	if len(patterns) == 0 {
		patterns = []string{"./**"}
	}

	files, err := ResolveSource(dir, patterns)
	if err != nil {
		return nil, "", fmt.Errorf("resolving source: %w", err)
	}

	// Verify script is in the resolved file list.
	scriptRel := filepath.Clean(tf.App.Script)
	found := false
	for _, f := range files {
		if f == scriptRel {
			found = true
			break
		}
	}
	if !found {
		return nil, "", fmt.Errorf("script %q is not matched by any source pattern", tf.App.Script)
	}

	// Always include the Towerfile. Add it if not already in the set.
	hasTowerfile := false
	for _, f := range files {
		if f == "Towerfile" {
			hasTowerfile = true
			break
		}
	}
	if !hasTowerfile {
		files = append(files, "Towerfile")
	}

	// Build tar.gz into a buffer, computing SHA256 as we write.
	var buf bytes.Buffer
	hash := sha256.New()
	w := io.MultiWriter(&buf, hash)

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	for _, rel := range files {
		absPath := filepath.Join(dir, rel)

		info, err := os.Lstat(absPath)
		if err != nil {
			return nil, "", fmt.Errorf("stat %q: %w", rel, err)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, "", fmt.Errorf("creating tar header for %q: %w", rel, err)
		}
		// Use the relative path (forward slashes) as the archive name.
		header.Name = filepath.ToSlash(rel)

		// Handle symlinks: store as symlink entry, don't follow.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(absPath)
			if err != nil {
				return nil, "", fmt.Errorf("reading symlink %q: %w", rel, err)
			}
			header.Typeflag = tar.TypeSymlink
			header.Linkname = target
			if err := tw.WriteHeader(header); err != nil {
				return nil, "", fmt.Errorf("writing symlink header for %q: %w", rel, err)
			}
			continue
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, "", fmt.Errorf("writing header for %q: %w", rel, err)
		}

		f, err := os.Open(absPath)
		if err != nil {
			return nil, "", fmt.Errorf("opening %q: %w", rel, err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return nil, "", fmt.Errorf("writing %q: %w", rel, err)
		}
		f.Close()
	}

	if err := tw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing gzip: %w", err)
	}

	return &buf, hex.EncodeToString(hash.Sum(nil)), nil
}
