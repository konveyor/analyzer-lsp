//go:build windows

package provider

import (
	"path/filepath"
	"strings"
)

// TranslatePath applies the first matching PathMapping to the given path.
// On Windows, path comparison normalizes separators to forward slashes and
// performs case-insensitive matching to handle drive letter differences
// (e.g., C:\ vs c:\). Matching is done on path boundaries to avoid partial
// directory name matches.
// If no mapping matches, the original path is returned unchanged.
func TranslatePath(path string, mappings []PathMapping) string {
	normalizedPath := normalizePath(path)
	for _, m := range mappings {
		normalizedFrom := normalizePath(m.From)
		if hasPathPrefix(normalizedPath, normalizedFrom) {
			remainder := path[len(m.From):]
			return filepath.Join(m.To, remainder)
		}
	}
	return path
}

// normalizePath converts backslashes to forward slashes and lowercases the
// string for case-insensitive comparison on Windows.
func normalizePath(path string) string {
	return strings.ToLower(filepath.ToSlash(path))
}

// hasPathPrefix checks whether path starts with prefix on a path boundary.
// Both path and prefix should already be normalized before calling.
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
