// Package sudocodefile handles the .sudocode file/dir indirection,
// following the same pattern as Git's gitdir: mechanism.
package sudocodefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// FileName is the name of the .sudocode file or directory.
	FileName = ".sudocode"
	// Prefix is the required prefix in a .sudocode file.
	Prefix = "sudocodedir: "
)

// ErrNotFound indicates no .sudocode file or directory exists.
var ErrNotFound = errors.New("sudocodefile: .sudocode not found")

// ErrMalformed indicates the .sudocode file has invalid content.
var ErrMalformed = errors.New("sudocodefile: malformed .sudocode file")

// ResolveSudocodeDir resolves the sudocode data directory for a repo.
//
// If .sudocode is a directory, it returns its path directly.
// If .sudocode is a file, it reads the sudocodedir: line and resolves the path.
// Relative paths are resolved relative to repoPath.
func ResolveSudocodeDir(repoPath string) (string, error) {
	p := filepath.Join(repoPath, FileName)

	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("sudocodefile: stat: %w", err)
	}

	if info.IsDir() {
		return p, nil
	}

	// It's a file — read and parse
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("sudocodefile: read: %w", err)
	}

	content := strings.TrimRight(string(data), "\n\r")
	if !strings.HasPrefix(content, Prefix) {
		return "", fmt.Errorf("%w: missing %q prefix", ErrMalformed, Prefix)
	}

	dir := strings.TrimPrefix(content, Prefix)
	if dir == "" {
		return "", fmt.Errorf("%w: empty path after prefix", ErrMalformed)
	}

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoPath, dir)
	}

	return filepath.Clean(dir), nil
}

// WriteSudocodeFile writes a .sudocode file with a sudocodedir: line
// pointing to the given sudocode data directory.
func WriteSudocodeFile(repoPath, sudocodeDir string) error {
	p := filepath.Join(repoPath, FileName)
	content := Prefix + sudocodeDir + "\n"
	return os.WriteFile(p, []byte(content), 0o644)
}
