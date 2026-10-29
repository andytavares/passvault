# Installation and management

## Requirements

- macOS (setup stops on any other OS).
- [Homebrew](https://brew.sh) on `PATH`.
- zsh as your shell (pvault writes to `~/.zshrc` only).
- The 1Password desktop app with **Settings > Developer > Integrate with 1Password CLI**
  turned on, so `op` can sign in through the app.
- Go is installed by `install.sh` through Homebrew if missing.

## Install

```sh
git clone git@github.com:andytavares/passvault.git
cd passvault
./install.sh
```

`install.sh`:

1. Checks for macOS and Homebrew.
2. Runs `brew install go` when `go` is missing (and remembers it, so uninstall can remove it).
3. Builds the binary to `~/.local/bin/pvault`, stamping the version from `git describe`.
4. Prints `Add ~/.local/bin to your PATH` when it is not on `PATH`.
5. Runs `pvault setup --source <this clone>`. Any extra arguments go to setup, for
   example `./install.sh --dry-run`.

Keep the clone: `pvault upgrade` pulls and rebuilds from it.

## What setup does

`pvault setup` runs nine steps in order. Every step checks first and only acts when
needed, so running setup again is safe. Each step prints one line:

| Glyph | Meaning |
|---|---|
| `✓` | already in place, nothing done |
| `+` | applied a change |
| `-` | skipped (dry run) |
| `!` | warning, worth reading |
| `✕` | failed |

| Step | Checks | Changes when missing |
|---|---|---|
| `darwin + brew` | macOS and `brew` on `PATH` | none; fails instead |
| `brew packages` | `pass`, `gnupg`, `pinentry-mac`, `1password-cli` (cask) | `brew install` each missing one |
| `gpg-agent.conf` | a `pinentry-program` line | adds a managed block pointing at `pinentry-mac`, restarts `gpg-agent`. A line you wrote yourself is left alone |
| `gpg key` | any GPG secret key | asks, then runs `gpg --quick-generate-key "<name> <email>" future-default default 2y` (name and email default to your git config). **The key expires in 2 years** |
| `pass store` | `~/.password-store/.gpg-id` | `pass init <fingerprint>`; warns when `.gpg-id` names a key you have no secret key for |
| `store git` | `~/.password-store/.git` | `pass git init` (history and rollback need it) |
| `mapping.toml` | the mapping file | writes a commented example, mode `0600` |
| `zshrc block` | the managed block in `~/.zshrc` | adds the block below; warns about lines elsewhere in `~/.zshrc` that call `pass` |
| `1password` | `op whoami` | none; warns when op is not signed in |

The `~/.zshrc` block:

```sh
# >>> pvault >>>
[[ -x '/Users/you/.local/bin/pvault' ]] && eval "$('/Users/you/.local/bin/pvault' env --zsh)"
# <<< pvault <<<
```

Before setup first edits `~/.zshrc`, it saves the original to
`~/.zshrc.pvault-<UTC timestamp>`. Everything outside the markers stays byte for byte.

Setup records what it installed or created in `~/.local/state/pvault/manifest.json`.
Uninstall reads that file, so it only removes what pvault added.

Preview without changing anything:

```sh
pvault setup --dry-run
```

## Check the install: `pvault doctor`

```sh
pvault doctor
```

| Check | Fails (`✕`) when | Warns (`!`) when |
|---|---|---|
| `pass version`, `gpg version`, `op version` | the tool is missing or its version line is unexpected | |
| `op sign-in` | | op is not signed in |
| `pinentry-program` | | no pinentry line in `gpg-agent.conf` |
| `pass store` | no `.gpg-id`, or no secret key for it | |
| `store git` | | the store is not a git repo |
| `mapping.toml` | the file does not parse or validate | the file is missing; one warning per `global` secret with `env`, because it reaches every child process |
| `zshrc` | | a line outside the block calls `pass` |

Exit code `1` when any check fails, else `0`.

## Upgrade

```sh
pvault upgrade             # pvault and the brew tools
pvault upgrade --no-tools  # pvault only
```

Upgrade:

1. Runs `git pull --ff-only` in the clone recorded at install time. With no recorded
   clone, it clones `https://github.com/andytavares/passvault.git` into
   `~/.local/state/pvault/source` and records that one.
2. Builds the new binary next to the old one and swaps it in with one rename.
3. Runs `brew upgrade` for each of `pass`, `gnupg`, `pinentry-mac`, `1password-cli`
   that is installed (unless `--no-tools`). A failed upgrade is a warning, not an error.
4. Runs the **new** binary's `pvault setup`, which applies any migrations.

## Uninstall

Always look at the plan first:

```sh
pvault uninstall --dry-run
```

It prints `Will remove` and `Will keep` lists. By default uninstall removes:

- each brew package setup installed, unless another installed package that pvault
  did not install still needs it;
- the managed blocks in `~/.zshrc` and `gpg-agent.conf`;
- `~/.config/pvault` (the mapping file);
- `~/.local/state/pvault` (manifest and sync state);
- `~/.local/bin/pvault`.

It keeps by default:

- packages that were already installed before setup;
- the GPG key, even the one setup created;
- the pass store and its history;
- the `~/.zshrc.pvault-*` backups.

| Flag | Effect |
|---|---|
| `--keep-config` | keep `~/.config/pvault` |
| `--purge-key` | also delete the GPG key, only if setup created it |
| `--purge-vault` | also delete the pass store. You must type the store path (full or `~/…`) to confirm |
| `--yes` | skip the `Continue? [y/N]` prompt (not the vault path prompt) |
| `--dry-run` | print the plan only |

> `--purge-vault` deletes every secret in the store, including entries pvault did not
> create, and its git history. There is no undo.

## Paths and environment variables

pvault honours these variables. Unset means the default.

| Variable | Default | Used for |
|---|---|---|
| `XDG_CONFIG_HOME` | `~/.config` | `pvault/mapping.toml` |
| `XDG_STATE_HOME` | `~/.local/state` | `pvault/manifest.json`, `pvault/state.json` |
| `PASSWORD_STORE_DIR` | `~/.password-store` | the pass store |
| `GNUPGHOME` | `~/.gnupg` | `gpg-agent.conf` |
| `EDITOR` | `vi` | the TUI's mapping editor. A single program name, no arguments |

The binary is always `~/.local/bin/pvault`. Set the same variables in every shell,
or pvault will look at a different store or mapping.
