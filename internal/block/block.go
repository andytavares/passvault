// Package block edits the pvault-managed block inside a shared text file
// (~/.zshrc, gpg-agent.conf) and leaves every other byte unchanged.
package block

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// Begin opens the managed block.
	Begin = "# >>> pvault >>>"
	// End closes the managed block.
	End = "# <<< pvault <<<"
)

// Render returns the full block: markers around body, newline-terminated.
func Render(body string) string {
	return Begin + "\n" + strings.TrimRight(body, "\n") + "\n" + End + "\n"
}

// Upsert replaces the managed block in content, or appends it when absent.
// Content outside the markers is returned byte for byte.
func Upsert(content, body string) (string, error) {
	start, end, err := find(content)
	if err != nil {
		return "", err
	}
	if start < 0 {
		separator := ""
		if content != "" && !strings.HasSuffix(content, "\n") {
			separator = "\n"
		}
		return content + separator + Render(body), nil
	}
	return content[:start] + Render(body) + content[end:], nil
}

// Remove deletes the managed block and reports whether one was present.
func Remove(content string) (string, bool, error) {
	start, end, err := find(content)
	if err != nil || start < 0 {
		return content, false, err
	}
	return content[:start] + content[end:], true, nil
}

// Has reports whether content contains a managed block.
func Has(content string) bool {
	start, _, err := find(content)
	return err == nil && start >= 0
}

// find returns the byte range of the block including the End line's newline,
// or -1, -1 when there is no block.
func find(content string) (int, int, error) {
	start := strings.Index(content, Begin)
	if start < 0 {
		if strings.Contains(content, End) {
			return -1, -1, errors.New("found end marker without begin marker")
		}
		return -1, -1, nil
	}
	endRel := strings.Index(content[start:], End)
	if endRel < 0 {
		return -1, -1, errors.New("found begin marker without end marker")
	}
	end := start + endRel + len(End)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return start, end, nil
}

// UpsertFile applies Upsert to path. It backs up an existing file that has
// no block yet to path.pvault-<timestamp> and returns that backup path.
func UpsertFile(path, body string, now time.Time) (string, error) {
	content, err := readOptional(path)
	if err != nil {
		return "", err
	}
	updated, err := Upsert(content, body)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if updated == content {
		return "", nil
	}
	backup := ""
	if content != "" && !Has(content) {
		backup = path + ".pvault-" + now.UTC().Format("20060102T150405")
		if err := os.WriteFile(backup, []byte(content), 0o600); err != nil {
			return "", err
		}
	}
	return backup, writePreservingMode(path, updated)
}

// RemoveFile applies Remove to path and reports whether it changed the file.
func RemoveFile(path string) (bool, error) {
	content, err := readOptional(path)
	if err != nil {
		return false, err
	}
	updated, removed, err := Remove(content)
	if err != nil || !removed {
		return false, err
	}
	return true, writePreservingMode(path, updated)
}

func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

func writePreservingMode(path, content string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, []byte(content), mode)
}
