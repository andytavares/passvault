package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// appendEvent holds an exclusive flock while it reads the last line and
// writes the next one, so concurrent pvault processes keep the chain intact.
func appendEvent(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	last, err := lastLine(file)
	if err != nil {
		return err
	}
	event.Prev = chainHash(splitLines(last))
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	return err
}

// tailChunkBytes is how much lastLine reads per step back from the end.
const tailChunkBytes = 4096

// lastLine returns the file's last line, without its newline, reading back
// from the end so each append costs the same however long the log grows.
func lastLine(file *os.File) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	end := info.Size()
	var tail []byte
	for start := end; start > 0; {
		start = max(0, start-tailChunkBytes)
		chunk := make([]byte, end-start-int64(len(tail)))
		if _, err := file.ReadAt(chunk, start); err != nil {
			return nil, err
		}
		tail = append(chunk, tail...)
		if newline := bytes.LastIndexByte(bytes.TrimSuffix(tail, []byte("\n")), '\n'); newline >= 0 {
			return tail[newline+1:], nil
		}
	}
	return tail, nil
}

// chainHash is the Prev value for the line that follows lines.
func chainHash(lines [][]byte) string {
	if len(lines) == 0 {
		return ""
	}
	sum := sha256.Sum256(lines[len(lines)-1])
	return hex.EncodeToString(sum[:])
}

func splitLines(content []byte) [][]byte {
	if len(content) == 0 {
		return nil
	}
	return bytes.Split(bytes.TrimSuffix(content, []byte("\n")), []byte("\n"))
}

func readLines(path string) ([][]byte, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return splitLines(content), err
}

// Read returns every event in the log at path; a missing file yields none.
func Read(path string) ([]Event, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, line := range lines {
		var event Event
		// A torn line must not hide the rest of the log; Verify reports it.
		if json.Unmarshal(line, &event) == nil {
			events = append(events, event)
		}
	}
	return events, nil
}

// Verify recomputes the hash chain from line 1 and returns the event count.
// The error names the first line that does not parse or does not chain.
func Verify(path string) (int, error) {
	lines, err := readLines(path)
	if err != nil {
		return 0, err
	}
	for number, line := range lines {
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return 0, fmt.Errorf("line %d: %w", number+1, err)
		}
		if event.Prev != chainHash(lines[:number]) {
			return 0, fmt.Errorf("line %d: chain broken", number+1)
		}
	}
	return len(lines), nil
}
