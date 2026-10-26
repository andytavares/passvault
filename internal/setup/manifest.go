// Package setup runs the idempotent install steps, records them in a
// manifest, and reverses them on uninstall.
package setup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Package is one brew package that setup installed.
type Package struct {
	Name string `json:"name"`
	Cask bool   `json:"cask,omitempty"`
}

// Manifest records what setup installed so uninstall can reverse it.
type Manifest struct {
	SchemaVersion    int       `json:"schema_version"`
	SourceDir        string    `json:"source_dir,omitempty"`
	Packages         []Package `json:"packages,omitempty"`
	CreatedKey       string    `json:"created_key,omitempty"`
	Blocks           []string  `json:"blocks,omitempty"`
	Backups          []string  `json:"backups,omitempty"`
	InitializedStore bool      `json:"initialized_store,omitempty"`
	InitializedGit   bool      `json:"initialized_git,omitempty"`
	InstalledAt      time.Time `json:"installed_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

// LoadManifest reads path. A missing file is a zero manifest, not an error.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Save writes the manifest to path, mode 0600.
func (m *Manifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// HasPackage reports whether the manifest lists the package.
func (m *Manifest) HasPackage(name string) bool {
	for _, pkg := range m.Packages {
		if pkg.Name == name {
			return true
		}
	}
	return false
}

// Add appends the package unless it is already listed, keeping order.
func (m *Manifest) Add(pkg Package) {
	if !m.HasPackage(pkg.Name) {
		m.Packages = append(m.Packages, pkg)
	}
}
