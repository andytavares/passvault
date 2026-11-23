# TUI

Run `pvault` with no arguments in a terminal. It takes the whole screen and
gives it back when you quit.

```
 pvault ─ /Users/you/.password-store ─ 3 mapped · 1 unmanaged · 0 orphan  op ✓
  Vault   Sync   History   Mapping   Env   Audit
──────────────────────────────────────────────────────────────────────────
Entries                                           │ pagerduty-api-key
▸ pagerduty-api-key            ● synced 2h        │
  notes/wifi                   ○ unmanaged        │ 1Password  op://Private/PagerDuty API Key/credential
                                                  │ Value      ••••••••••••••••  press enter to reveal
──────────────────────────────────────────────────────────────────────────
 copied pagerduty-api-key (clears in 45 s)
 enter show/hide • c copy • h history • p holders • s sync • tab next view • ? help • q quit
```

The header shows the store, entry counts and whether `op` is signed in (`op ✓` /
`op ✕`). Below it, the tab bar highlights the current view. The bottom line lists
the keys that work right now; errors and notices appear just above it. Prompts
open in a box in the middle of the screen.

## Keys in every view

| Key | Action |
|---|---|
| `tab` / `→` | next view |
| `shift+tab` / `←` | previous view |
| `?` | toggle full help |
| `q`, `ctrl+c` | quit (ignored while a sync runs) |
| `↑`/`k`, `↓`/`j` | move |

Switching view hides any revealed value.

## Vault

Left: every entry. Right: the selected entry's 1Password reference, env, scope,
masked value, last sync and git history.

| Glyph | Meaning |
|---|---|
| `●` | mapped and synced |
| `◐` | mapped, never synced |
| `○` | unmanaged (in pass only) |
| `✕` | orphan |
| `◉` | audited: `enter` and `c` on it write an audit event |

| Key | Action |
|---|---|
| `enter` | show the value for 10 seconds; `enter` again hides it |
| `c` | copy to the clipboard with `pass show -c`; pass clears it after 45 seconds |
| `h` | open History filtered to the entry |
| `p` | list the processes started with the entry's env variable; `p` again, moving or switching view hides the list. Works only on an enabled `global` entry with an env variable |
| `n` | new pass entry: type a name, `enter`, type the value (masked), `enter`. It is unmanaged: not added to the mapping |
| `d` | delete the entry: type its exact name to confirm |
| `s` | start a sync and switch to the Sync view |
| `r` | refresh: re-read 1Password and the list |

The footer lists `n`, `d` and `r` only in the full help (`?`).

`p` shows `Held by    N processes started with ENV` and one line per process
with its pid and command. It finds only processes whose launch environment sets
the variable. macOS hides Apple's own programs, so they never appear.

In any prompt, `esc` cancels and `backspace` deletes.

## Sync

`s` starts a sync here too. Shows `running…` while a sync runs, then a summary notice:
`synced: N changed · N unchanged · N skipped`.

A differing entry opens a conflict box with both lengths:

| Key | Action |
|---|---|
| `o` | overwrite pass with the 1Password value |
| `s` | skip this entry |
| `enter` | show both values for 10 seconds |
| `esc` | stop the sync |

The TUI sync writes its audit events with via `sync`, so the Audit view hides them
by default.

There is no "overwrite all" key on purpose. The TUI sync has no prune or
skip-conflicts option; use `pvault sync --prune` or `--skip-conflicts`.

## History

Lists sync tags and store commits. `enter` on a commit previews the rollback (entries
restored and removed) and runs it only after you type `rollback` and press `enter`.

| Key | Action |
|---|---|
| `enter` | roll back the store, or only the filtered entry, to the commit |
| `f` | step the entry filter through each Vault entry, then back to the whole store |

Filtered to one entry (`f`, or `h` in Vault), the view lists every commit that
changed the entry with its kind: `added`, `changed`, `removed` or `rolled back`.
The detail pane shows the commit, its tags and, when the audit log has the write
event of that commit, who made it and from which process. `enter` previews and
runs `Rollback <entry> to <commit>`; other entries stay as they are.

## Mapping

Lists every `[[secret]]` with its reference, and any validation errors in red.

| Key | Action |
|---|---|
| `n` | new entry from 1Password (below) |
| `e` | open the mapping in `$EDITOR` (default `vi`); reloads and re-validates on exit |

Add from 1Password:

1. Pick an item (`↑`/`↓`, `enter`). The list comes from `op item list` and
   scrolls to the cursor. Type to search titles (case-insensitive, `backspace`
   deletes); `←`/`→` steps the vault filter through all vaults and each vault.
   While the picker is open, letters, `q` and `←`/`→` go to the search and filter,
   not to the global keys.
2. Pick a field. Labels and references only; values are never loaded.
3. Edit the pass name (defaults to the item title in lower-case-with-dashes), `enter`.
4. Type an env name (upper-cased for you) or leave it empty, `enter`.

The entry is appended with scope `global`. `esc` closes the picker. Run a sync to
copy the value.

## Env

Lists every entry with its env variable, scope, state (`● on`, `○ off`) and source
(`1Password`, or `local` for an entry with no `ref`). Changes go to the mapping file;
only `n` writes a pass entry. Scope and commands are edited in the
Mapping view (`e`).

| Key | Action |
|---|---|
| `n` | create a pass entry with an env variable: type the entry name, the env name and the value. The entry gets a local `[[secret]]`. An existing entry name is refused; select it and press `a` |
| `a` | add or rename the env variable (upper-cased for you). On an entry with no `[[secret]]`, adds a local one |
| `space` | turn the variable on or off (`enabled = false`) |
| `d` | remove the env variable: type its name to confirm. A local entry's `[[secret]]` is removed; a 1Password entry keeps syncing |
| `A` | turn the audit log on or off for the entry (`audit = true`) |

Open a new shell to load the change; a running shell keeps the variables it already has.

## Audit

Lists audit events, newest first. The detail pane shows who read or changed the
entry, with what process, and when. The view reads only the audit log; it never
decrypts.

| Key | Action |
|---|---|
| `j`/`k` | move |
| `f` | step the entry filter through each entry in the log, then back to all entries |
| `a` | show or hide routine rows: every `compare` and every event via `sync` |

Routine rows are hidden by default. The header then says
`N of M events · K hidden`.

## Known issues

- **Sync and check show no per-entry lines.** The Sync view shows only `running…`
  and the summary; `r` only refreshes the entry list and says `check complete`. Use
  `pvault sync` or `pvault check` to see each entry.
