# Command reference

Errors print as `pvault: <message>` on stderr. Exit code is `1` for any error unless
a command lists otherwise.

| Command | Does |
|---|---|
| [`pvault`](#pvault) | open the TUI |
| [`sync`](#sync) | copy secrets from 1Password into pass |
| [`check`](#check) | compare without writing |
| [`status`](#status) | list entries and last sync time |
| [`env`](#env) | print the zsh exports and wrappers |
| [`run`](#run) | run one command with secrets |
| [`history`](#history) | list store commits and sync tags |
| [`rollback`](#rollback) | restore the store or one entry |
| [`mapping`](#mapping) | create, locate, validate and add to the mapping file |
| [`setup`](#setup) | install and configure |
| [`doctor`](#doctor) | check the install |
| [`upgrade`](#upgrade) | rebuild from source and migrate |
| [`uninstall`](#uninstall) | remove what setup added |
| [`version`](#version) | print the version |
| `completion` | print a shell completion script (`pvault completion zsh`) |

## pvault

```
pvault
```

Opens the [TUI](tui.md) when both stdin and stdout are terminals, else prints help.

## sync

```
pvault sync [NAME…] [--dry-run] [--prune] [--skip-conflicts]
```

Reads each mapped secret from 1Password and writes it into pass. `NAME` limits the
run to those pass paths.

| Flag | Effect |
|---|---|
| `--dry-run` | print the plan, write nothing, skip the dirty-store check |
| `--prune` | delete orphans (entries pvault synced before that left the mapping) from pass. Applies to all orphans, even with `NAME` |
| `--skip-conflicts` | never prompt or overwrite; count differing entries as skipped |

Output lines: `+ added`, `= unchanged`, `~ differs`, `? orphan`, `✕ failed`. The
summary is one of:

- `Tagged sync-<timestamp> · N changed · N unchanged · N orphan`
- `No changes · …` (nothing written)
- `No changes tagged · …` (dry run, or the store is not a git repo)

Conflict prompt: `[o]verwrite [s]kip [v]reveal [q]uit`. See
[usage](usage.md#resolve-a-conflict).

| Exit | When |
|---|---|
| 0 | done |
| 1 | `--skip-conflicts` and an entry differs; or an error (no mapping, op not signed in, dirty store) |
| 2 | at least one entry failed |

State is saved to `state.json` after every non-dry run.

## check

```
pvault check [NAME…]
```

Same comparison as sync, never writes, never prompts. Same output lines. Orphans are
always listed.

Exit `1` with `N entries not in sync` when any line is not `unchanged`.

## status

```
pvault status [--json]
```

Lists every mapped entry and every pass entry. Reads only the mapping, `state.json`
and file names in the store: no 1Password, no decryption.

```
NAME                         KIND           ENV                SCOPE    LAST SYNC
github-token                 mapped         GITHUB_TOKEN       global   3h
notes/wifi                   unmanaged      -                  -        never
```

`LAST SYNC` is the time since the value was last written, as `Ns`, `Nm`, `Nh`, `Nd`
or `never`. An unchanged sync does not reset it. `--json` prints an array of
`{Name, Kind, Env, Scope, SyncedAt, Tag}`. Kinds are explained in
[usage](usage.md#see-what-is-in-sync).

## env

```
pvault env [--zsh]
```

Prints the zsh script `~/.zshrc` evaluates: one `export` per global entry with `env`,
then one wrapper function per command named by command-scope entries. `--zsh` is
accepted and changes nothing (zsh is the only shell).

Never fails: a missing mapping prints nothing; an unreadable entry, or a value with
a NUL byte, is skipped with `pvault: skipped NAME: reason` on stderr.

## run

```
pvault run [--entry NAME]… -- CMD [ARGS…]
```

Runs `CMD` with secrets added to its environment. Without `--entry`, uses every
mapped entry that has `env`, whatever its scope. `CMD` inherits the terminal.

Exits with `CMD`'s exit code. Fails before starting `CMD` when an entry is unknown,
unreadable, or contains a NUL byte.

## history

```
pvault history [NAME] [--limit N]
```

Without `NAME`: a `Sync tags:` line (newest first), then commits. With `NAME`: only
commits touching that entry. `--limit` defaults to 20; `0` means all.

Line format: `SHORT  YYYY-MM-DD HH:MM  subject  [tags]`.

## rollback

```
pvault rollback REV [--entry NAME] [--yes]
```

`REV` is anything git accepts: a sync tag, a hash, `HEAD~2`. Restores the whole store
(or only `--entry`) to `REV` and commits `pvault rollback to REV`.

1. Prints `Restores N entries, removes M (names)`, or `Nothing to roll back`.
2. Asks `Continue? [y/N]` unless `--yes`.
3. Refuses when the store is dirty or `.gpg-id` at `REV` differs from now.
4. Prints `Committed pvault rollback to REV · undo with pvault rollback HEAD~1`.

## mapping

```
pvault mapping init
pvault mapping path
pvault mapping validate
pvault mapping add --pass P --ref R [--env E] [--scope S] [--command C]…
```

| Subcommand | Does |
|---|---|
| `init` | writes the commented example (mode `0600`) if no file exists; never overwrites |
| `path` | prints the mapping file path |
| `validate` | prints `OK, N entries`, or every problem and exits `1` |
| `add` | appends one `[[secret]]`. `--pass` and `--ref` are required. `--command` repeats. Fails without changing the file if the result would be invalid |

Field rules are in [mapping.md](mapping.md).

## setup

```
pvault setup [--dry-run] [--source DIR]
```

Runs the nine install steps. `--source` records the clone `upgrade` pulls from;
`install.sh` passes it. Details in [install.md](install.md#what-setup-does).

## doctor

```
pvault doctor
```

Prints one `✓`/`!`/`✕` line per check. Exit `1` when any check is `✕`. Details in
[install.md](install.md#check-the-install-pvault-doctor).

## upgrade

```
pvault upgrade [--no-tools]
```

Pulls the source clone, rebuilds `~/.local/bin/pvault`, upgrades the brew tools
(unless `--no-tools`), then runs the new binary's `setup`.

## uninstall

```
pvault uninstall [--dry-run] [--yes] [--keep-config] [--purge-key] [--purge-vault]
```

Prints the remove/keep plan and asks before acting. Flags in
[install.md](install.md#uninstall).

## version

```
pvault version
```

Prints `pvault <git describe>`, or `pvault dev` for a plain `go build`.
