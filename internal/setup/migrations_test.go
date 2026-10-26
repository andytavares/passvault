package setup_test

import (
	"context"
	"os"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/setup"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

func TestManifestLoadMissingIsZeroValue(t *testing.T) {
	env := testenv.New(t)
	manifest, err := setup.LoadManifest(env.Paths.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 0 || len(manifest.Packages) != 0 || manifest.SourceDir != "" {
		t.Fatalf("got %+v", manifest)
	}
}

func TestManifestRoundTripKeepsOrderAndMode(t *testing.T) {
	env := testenv.New(t)
	manifest := &setup.Manifest{SourceDir: "/src"}
	manifest.Add(setup.Package{Name: "pass"})
	manifest.Add(setup.Package{Name: "1password-cli", Cask: true})
	manifest.Add(setup.Package{Name: "pass"})
	if err := manifest.Save(env.Paths.Manifest()); err != nil {
		t.Fatal(err)
	}
	loaded, err := setup.LoadManifest(env.Paths.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if got := packageNames(loaded); len(got) != 2 || got[0] != "pass" || got[1] != "1password-cli" {
		t.Fatalf("packages %v", got)
	}
	if info, _ := os.Stat(env.Paths.Manifest()); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestMigrationsRunInOrderOnce(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{SchemaVersion: 0})
	var order []int
	list := []setup.Migration{
		{To: 2, Apply: func(context.Context, *setup.Installer) error { order = append(order, 2); return nil }},
		{To: 1, Apply: func(context.Context, *setup.Installer) error { order = append(order, 1); return nil }},
		{To: 3, Apply: func(context.Context, *setup.Installer) error { order = append(order, 3); return nil }},
	}
	if err := inst.RunMigrations(context.Background(), list, 3); err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order %v", order)
	}
	if inst.Manifest.SchemaVersion != 3 {
		t.Fatalf("schema %d", inst.Manifest.SchemaVersion)
	}
	order = nil
	if err := inst.RunMigrations(context.Background(), list, 3); err != nil {
		t.Fatal(err)
	}
	if len(order) != 0 {
		t.Fatalf("reran %v", order)
	}
}
