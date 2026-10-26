package setup

import (
	"context"
	"sort"
)

// CurrentSchema is the newest manifest schema this binary writes.
const CurrentSchema = 1

// Migration upgrades the manifest and the files it tracks to version To.
type Migration struct {
	To    int
	Apply func(ctx context.Context, in *Installer) error
}

// migrations is the ordered list of schema upgrades; empty at schema 1.
var migrations = []Migration{}

// RunMigrations runs every migration above the stored schema, oldest first.
func (in *Installer) RunMigrations(ctx context.Context, list []Migration, current int) error {
	ordered := make([]Migration, len(list))
	copy(ordered, list)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].To < ordered[j].To })
	for _, migration := range ordered {
		if migration.To <= in.Manifest.SchemaVersion {
			continue
		}
		if err := migration.Apply(ctx, in); err != nil {
			return err
		}
		in.Manifest.SchemaVersion = migration.To
	}
	in.Manifest.SchemaVersion = current
	return nil
}
