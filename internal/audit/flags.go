package audit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// SyncFlags compares audited with the names saved at flagsPath, records
// audit-on for each added name and audit-off for each removed one, then
// saves audited. It does nothing when the names are unchanged.
func SyncFlags(r *Recorder, flagsPath string, audited []string) error {
	previous, err := loadFlags(flagsPath)
	if err != nil {
		return err
	}
	added := missingFrom(previous, audited)
	removed := missingFrom(audited, previous)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	for _, name := range added {
		if err := r.Record(name, ActionAuditOn, ""); err != nil {
			return err
		}
	}
	for _, name := range removed {
		if err := r.Record(name, ActionAuditOff, "audit flag removed"); err != nil {
			return err
		}
	}
	return saveFlags(flagsPath, audited)
}

// missingFrom returns the names in candidates that are not in base.
func missingFrom(base, candidates []string) []string {
	var missing []string
	for _, name := range candidates {
		if !slices.Contains(base, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func loadFlags(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	return names, json.Unmarshal(data, &names)
}

func saveFlags(path string, names []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// WasAudited reports whether name was audited at the last SyncFlags. It keeps
// an entry audited while it leaves the mapping, and when the mapping is broken.
func WasAudited(flagsPath, name string) bool {
	names, err := loadFlags(flagsPath)
	return err == nil && slices.Contains(names, name)
}
