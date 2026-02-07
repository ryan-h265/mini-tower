package towerfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolveSource expands glob patterns relative to dir and returns a sorted,
// deduplicated list of file paths (relative to dir). Symlinks whose targets
// fall outside dir are rejected.
func ResolveSource(dir string, patterns []string) ([]string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving directory: %w", err)
	}

	seen := make(map[string]bool)
	var files []string

	for _, pattern := range patterns {
		if containsTraversal(pattern) {
			return nil, fmt.Errorf("source pattern %q escapes the project root", pattern)
		}

		// Determine whether the pattern explicitly targets dotfiles.
		// If no segment in the pattern starts with a literal dot (ignoring
		// the leading "./" current-dir prefix), we filter out dotfiles
		// from the matches.
		filterDot := !patternTargetsDotfiles(pattern)

		absPattern := filepath.Join(absDir, filepath.FromSlash(pattern))

		matches, err := doublestar.FilepathGlob(absPattern)
		if err != nil {
			return nil, fmt.Errorf("expanding glob %q: %w", pattern, err)
		}

		for _, match := range matches {
			info, err := os.Lstat(match)
			if err != nil {
				return nil, fmt.Errorf("stat %q: %w", match, err)
			}

			// Skip directories — we only package files and symlinks.
			if info.IsDir() {
				continue
			}

			rel, err := filepath.Rel(absDir, match)
			if err != nil {
				return nil, fmt.Errorf("computing relative path for %q: %w", match, err)
			}

			// Exclude dotfiles unless the pattern explicitly targets them.
			if filterDot && hasDotSegment(rel) {
				continue
			}

			// For symlinks, verify the target stays within the project root.
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := filepath.EvalSymlinks(match)
				if err != nil {
					return nil, fmt.Errorf("resolving symlink %q: %w", match, err)
				}
				targetRel, err := filepath.Rel(absDir, target)
				if err != nil || strings.HasPrefix(targetRel, "..") {
					return nil, fmt.Errorf("symlink %q points outside the project root", match)
				}
			}

			if !seen[rel] {
				seen[rel] = true
				files = append(files, rel)
			}
		}
	}

	sort.Strings(files)
	return files, nil
}

// patternTargetsDotfiles returns true if any segment of the pattern (beyond
// the leading "./" current-dir prefix) starts with a literal dot.
func patternTargetsDotfiles(pattern string) bool {
	segments := strings.Split(filepath.ToSlash(pattern), "/")
	for i, seg := range segments {
		// Skip leading "." (current directory).
		if i == 0 && seg == "." {
			continue
		}
		if len(seg) > 0 && seg[0] == '.' {
			return true
		}
	}
	return false
}

// hasDotSegment returns true if any component of a relative path starts with
// a dot (i.e., is a dotfile or inside a dot-directory).
func hasDotSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if len(seg) > 0 && seg[0] == '.' {
			return true
		}
	}
	return false
}
