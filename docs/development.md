# Development

Go version and dependencies are pinned in `go.mod`.

## Make targets

| Target | Runs |
|---|---|
| `make build` | `go build` to `bin/pvault` with the `git describe` version |
| `make test` | `go test -race ./...` |
| `make cover` | coverage summary |
| `make lint` | `gofmt -l`, `go vet`, `shellcheck install.sh scripts/*.sh` |
| `make check` | lint, test, then `scripts/check-dep-age.sh` |

`check-dep-age.sh` fails when any module in the build graph was published less than
`MIN_AGE_DAYS` (default 14) days ago.

Tests call the real `pass`, `gpg` and `git` in throwaway homes built by
`internal/testenv`, so those tools must be installed. `op` and `brew` are fakes placed first on `PATH`.

## Layout

| Package | Role |
|---|---|
| `cmd/pvault` | `main`: runs the root command, maps `ExitCodeError` to the exit code |
| `internal/cli` | one file per cobra command; `app.go` holds shared dependencies |
| `internal/tui` | Bubble Tea UI; all logic in `Model` so tests drive `Update` without a terminal |
| `internal/syncer` | compare, sync, conflicts, state, status rows |
| `internal/store` | pass entries and the store's git history and rollback |
| `internal/opcli` | the `op` CLI wrapper |
| `internal/mapping` | parse, validate and append to `mapping.toml` |
| `internal/shellenv` | the `env --zsh` script and `run` environment |
| `internal/setup` | setup steps, manifest, migrations, upgrade, uninstall |
| `internal/block` | managed-block editing of shared files |
| `internal/paths` | every path, from environment variables |
| `internal/execx` | runs external commands; tests swap the runner |
| `internal/testenv` | isolated HOME, GPG key and pass store for tests |

## Adding a manifest migration

Append to `migrations` in `internal/setup/migrations.go` with `To` set to the new
schema number, and raise `CurrentSchema` to match. `setup` runs every migration
above the stored version, oldest first.
