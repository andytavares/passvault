# Usage guide

Every task below has a CLI form. Most also work in the [TUI](tui.md).

## Add a secret

You need its 1Password secret reference (`op://vault/item/field`). In the 1Password
app open the item, open the field's menu and choose **Copy Secret Reference**. More
ways in [mapping.md](mapping.md#getting-a-reference).

```sh
pvault mapping add \
  --pass pagerduty-api-key \
  --ref "op://Private/PagerDuty/credential" \
  --env PAGERDUTY_API_KEY
pvault sync
```

`mapping add` validates the new entry against the whole file before writing, then
appends a `[[secret]]` table and leaves the rest of the file untouched. You can also
edit `~/.config/pvault/mapping.toml` by hand (then run `pvault mapping validate`),
or open the Mapping tab in the TUI and press `n` to pick an item and field from 1Password.

### Choose who sees it

| You want | Mapping | Result |
|---|---|---|
| Every shell and tool sees it | `env = "X"` (scope `global` is the default) | `export X=…` in every new zsh |
| Only one or more commands see it | `env = "X"`, `scope = "command"`, `commands = ["gh"]` | a `gh()` shell function sets `X` for that call only |
| Nobody sees it automatically | no `env`, or `scope = "none"` | only in pass; read it with `pass show` |
| A one-off command sees it | any entry with `env` | `pvault run --entry NAME -- CMD` |

```sh
pvault mapping add --pass work/github-token --ref "op://Employee/GitHub PAT/token" \
  --env GITHUB_TOKEN --scope command --command gh --command gh-dash
```

## Sync

```sh
pvault sync              # every entry in the mapping
pvault sync github-token # only these entries
pvault sync --dry-run    # show what would happen, write nothing
```

1Password may ask for Touch ID. Each entry prints one line:

```
  + pagerduty-api-key  added
  = github-token  unchanged
  ~ work/aws-key  differs  1Password 40 chars · pass 41 chars
  ? old-token  orphan, no longer in mapping · run pvault sync --prune to delete
  ✕ broken  failed: op read op://Private/Broken/x: "Broken" isn't an item…
Tagged sync-20261025T143012 · 1 changed · 1 unchanged · 1 orphan
```

When anything changed and the store is a git repo, pvault tags the store
`sync-<UTC timestamp>`. pass itself commits each change.

Sync refuses to start when the store has uncommitted changes. Commit or discard them
first (`pass git status`).

### Resolve a conflict

`differs` means pass already has a different value. Sync never overwrites silently.
It asks for each one:

```
Overwrite pass entry? [o]verwrite [s]kip [v]reveal [q]uit:
```

- `o` writes the 1Password value into pass.
- `s` keeps the pass value and moves on.
- `v` prints both values once, then asks again.
- `q` stops; nothing after this entry is processed.

In a script with no input, the prompt reads end-of-file and quits. Use
`--skip-conflicts` there instead: it never overwrites and exits `1` if any entry differs.

### Rotate a secret

Rotate it in 1Password, then:

```sh
pvault sync NAME   # answer o at the prompt
exec zsh           # global vars are read at shell start
```

Command-scope variables need no new shell: the wrapper reads pass on each call.

### Remove a secret

Delete its `[[secret]]` table from the mapping. The pass entry stays and shows as an
orphan. To delete orphans from pass:

```sh
pvault sync --prune --dry-run   # check the list
pvault sync --prune
```

`--prune` removes **every** orphan, even when you name entries on the command line.

## See what is in sync

```sh
pvault check          # reads 1Password, writes nothing; exit 1 if anything is out of sync
pvault check NAME…    # only these entries (orphans are always listed)
pvault status         # no 1Password, no decryption; uses saved sync state
pvault status --json
```

`status` kinds:

| Kind | Meaning |
|---|---|
| `mapped` | in the mapping and in pass |
| `mapped-missing` | in the mapping, not yet in pass. Run `pvault sync` |
| `unmanaged` | in pass, not in the mapping, never synced by pvault. pvault leaves it alone |
| `orphan` | synced by pvault before, now gone from the mapping |

## Use secrets in the shell

New zsh sessions run `pvault env --zsh` through the `~/.zshrc` block. For a mapping
with one global and one command-scope entry it prints:

```sh
export PAGERDUTY_API_KEY='…'
gh() { GITHUB_TOKEN="$(command pass show 'work/github-token')" command gh "$@"; }
```

- Global values are decrypted once per shell start. gpg-agent may show a pinentry
  prompt the first time after login.
- An entry that cannot be read is skipped with a `pvault: skipped NAME: reason` line
  on stderr. A broken entry never breaks shell start.
- Changes to the mapping or to pass reach a shell only after `exec zsh` (for global
  vars) or right away (for command wrappers).

To load the environment into the current shell without restarting:

```sh
eval "$(pvault env --zsh)"
```

## Run one command with secrets

```sh
pvault run -- terraform plan                          # every entry that has env
pvault run --entry work/aws-key --entry github-token -- ./deploy.sh
```

- Values reach only that child process, whatever the entry's scope.
- Entries without `env` are ignored. An unknown entry name or an unreadable entry is
  an error, and the command does not start.
- pvault exits with the command's exit code.

## History and rollback

The store's git history records every change. Sync tags mark each sync.

```sh
pvault history                # sync tags, then the last 20 commits
pvault history NAME --limit 5 # commits touching one entry
```

```
Sync tags: sync-20261025T143012, sync-20261024T090100
a1b2c3d  2026-10-25 14:30  Add given password for pagerduty-api-key to store.  [sync-20261025T143012]
```

Roll back with a new commit (history is never rewritten):

```sh
pvault rollback sync-20261024T090100                 # whole store
pvault rollback a1b2c3d --entry work/aws-key         # one entry
pvault rollback HEAD~1 --yes                         # no prompt
```

pvault shows `Restores N entries, removes M (…)` and asks before acting. Entries
added after the target revision are removed. Undo a rollback with
`pvault rollback HEAD~1`.

Rollback refuses when:

- the store has uncommitted changes;
- the store was encrypted to a different GPG key at that revision.

The next `pvault sync` will see the rolled-back values as `differs` from 1Password.

## Move to another Mac

The mapping holds references only, no secrets. On the new Mac:

```sh
./install.sh
cp /path/to/old/mapping.toml "$(pvault mapping path)"
pvault mapping validate
pvault sync
```

## Use pvault in scripts

| Command | Exit 0 | Exit 1 | Exit 2 |
|---|---|---|---|
| `pvault sync --skip-conflicts` | all done | an entry differs, or any error | an entry failed to read or write |
| `pvault check` | all unchanged | anything to add, differing, orphaned or failed | |
| `pvault doctor` | no `✕` | at least one `✕` | |
| `pvault mapping validate` | valid | invalid | |
| `pvault run -- CMD` | the command's own exit code | | |

Plain `pvault` with stdin or stdout not a terminal prints help instead of opening
the TUI, so it never blocks a pipe.
