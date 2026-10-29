# TUI

Run `pvault` with no arguments in a terminal. It takes the whole screen and
gives it back when you quit.

```
 pvault ─ /Users/you/.password-store ─ 3 mapped · 1 unmanaged · 0 orphan  op ✓
  Vault   Sync   History   Mapping   Env
──────────────────────────────────────────────────────────────────────────
Entries                                           │ pagerduty-api-key
▸ pagerduty-api-key            ● synced 2h        │
  notes/wifi                   ○ unmanaged        │ 1Password  op://Private/PagerDuty API Key/credential
                                                  │ Value      ••••••••••••••••  press enter to reveal
──────────────────────────────────────────────────────────────────────────
 copied pagerduty-api-key (clears in 45 s)
 enter show/hide • c copy • n new • d delete • s sync • r refresh • tab next view • ? help • q quit
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

| Key | Action |
|---|---|
| `enter` | show the value for 10 seconds; `enter` again hides it |
| `c` | copy to the clipboard with `pass show -c`; pass clears it after 45 seconds |
| `n` | new pass entry: type a name, `enter`, type the value (masked), `enter`. It is unmanaged: not added to the mapping |
| `d` | delete the entry: type its exact name to confirm |
| `s` | start a sync and switch to the Sync view |
| `r` | refresh: re-read 1Password and the list |

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

There is no "overwrite all" key on purpose. The TUI sync has no prune or
skip-conflicts option; use `pvault sync --prune` or `--skip-conflicts`.

## History

Lists sync tags and store commits. `enter` on a commit previews the rollback (entries
restored and removed) and runs it only after you type `rollback` and press `enter`.

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
the pass entry itself is never changed. Scope and commands are edited in the
Mapping view (`e`).

| Key | Action |
|---|---|
| `a` | add or rename the env variable (upper-cased for you). On an entry with no `[[secret]]`, adds a local one |
| `space` | turn the variable on or off (`enabled = false`) |
| `d` | remove the env variable: type its name to confirm. A local entry's `[[secret]]` is removed; a 1Password entry keeps syncing |

Open a new shell to load the change; a running shell keeps the variables it already has.

## Known issues

- **Sync and check show no per-entry lines.** The Sync view shows only `running…`
  and the summary; `r` only refreshes the entry list and says `check complete`. Use
  `pvault sync` or `pvault check` to see each entry.
