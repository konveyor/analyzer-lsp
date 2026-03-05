//go:build !windows

package provider

import (
	"path/filepath"
	"strings"
)

// TranslatePath applies the first matching PathMapping to the given path.
// Matching is done on path boundaries to avoid partial directory name matches
// (e.g., From="/source" will not match "/source-other/foo").
// If no mapping matches, the original path is returned unchanged.
func TranslatePath(path string, mappings []PathMapping) string {
	for _, m := range mappings {
		if hasPathPrefix(path, m.From) {
			remainder := strings.TrimPrefix(path, m.From)
			return filepath.Join(m.To, remainder)
		}
	}
	return path
}

// hasPathPrefix checks whether path starts with prefix on a path boundary.
// It returns true when path equals prefix exactly, or when the character
// immediately after the prefix in path is a path separator.
func hasPathPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	// Exact match
	if len(path) == len(prefix) {
		return true
	}
	// Must be followed by a separator to avoid partial matches
	return path[len(prefix)] == '/'
}
