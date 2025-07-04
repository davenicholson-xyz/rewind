package watcher

import (
	"path/filepath"
	"testing"
)

func TestWatch_ShouldIgnore(t *testing.T) {
	tests := []struct {
		name           string
		watchPath      string
		ignorePatterns []string
		testPath       string
		expected       bool
	}{
		{
			name:           "ignore .git directory",
			watchPath:      "/home/user/project",
			ignorePatterns: []string{".git/"},
			testPath:       "/home/user/project/.git/config",
			expected:       true,
		},
		{
			name:           "ignore .rwignore file",
			watchPath:      "/home/user/project",
			ignorePatterns: []string{".git/", ".rwignore"},
			testPath:       "/home/user/project/.rwignore",
			expected:       true,
		},
		{
			name:           "ignore .tmp file",
			watchPath:      "/home/user/project",
			ignorePatterns: []string{".git/", ".rwignore", "*.tmp"},
			testPath:       "/home/user/project/something.tmp",
			expected:       true,
		},
		{
			name:           "not match readme.md file",
			watchPath:      "/home/user/project",
			ignorePatterns: []string{".git/", ".rwignore"},
			testPath:       "/home/user/project/readme.txt",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Watch{
				Path:           tt.watchPath,
				IgnorePatterns: tt.ignorePatterns,
			}

			result := w.ShouldIgnore(tt.testPath)
			if result != tt.expected {
				t.Errorf("ShouldIgnore() = %v, expected %v", result, tt.expected)

				// Add helpful debug info
				if relPath, err := filepath.Rel(tt.watchPath, tt.testPath); err == nil {
					t.Errorf("  Relative path: %s", relPath)
					t.Errorf("  Patterns: %v", tt.ignorePatterns)
				}
			}
		})
	}
}
