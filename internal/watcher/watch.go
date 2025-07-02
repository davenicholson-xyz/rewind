package watcher

import (
	"path/filepath"
	"strings"
)

type Watch struct {
	Path           string   `json:"path"`
	Active         bool     `json:"active"`
	IgnorePatterns []string `json:"-"`
	WatchDirs      []string `json:"-"`
}

func (w *Watch) ShouldIgnore(path string) bool {

	relPath, err := filepath.Rel(w.Path, path)
	if err != nil {
		return false
	}

	// Normalize path separators for consistent matching
	relPath = filepath.ToSlash(relPath)

	// Check each ignore pattern
	for _, pattern := range w.IgnorePatterns {
		// Normalize the pattern as well
		pattern = filepath.ToSlash(pattern)

		// Handle directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			// For directory patterns, check if the path starts with the pattern
			// or if any parent directory matches
			dirPattern := strings.TrimSuffix(pattern, "/")
			pathParts := strings.SplitSeq(relPath, "/")

			for part := range pathParts {
				if matched, _ := filepath.Match(dirPattern, part); matched {
					return true
				}
			}

			// Also check if the full relative path starts with the pattern
			if strings.HasPrefix(relPath+"/", pattern) {
				return true
			}
		} else {
			// For file patterns, check the filename and full path
			filename := filepath.Base(relPath)

			// Check if filename matches pattern
			if matched, _ := filepath.Match(pattern, filename); matched {
				return true
			}

			// Check if full relative path matches pattern
			if matched, _ := filepath.Match(pattern, relPath); matched {
				return true
			}

			// Check if any part of the path matches the pattern
			pathParts := strings.SplitSeq(relPath, "/")
			for part := range pathParts {
				if matched, _ := filepath.Match(pattern, part); matched {
					return true
				}
			}
		}
	}

	return false
}
