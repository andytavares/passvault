# TUI

Run `pvault` with no arguments in a terminal.

```
 pvault ─ /Users/you/.password-store ─ 3 mapped · 1 unmanaged · 0 orphan  op ✓
```

The header shows the store, entry counts and whether `op` is signed in (`op ✓` /
`op ✕`). The bottom line lists the keys for the current view; errors and notices
appear just above it.

## Keys in every view

| Key | Action |
|---|---|
| `1` `2` `3` `4` | Vault, Sync, History, Mapping |
| `tab` | next view |
| `?` | toggle full help |
| `q`, `ctrl+c` | quit (ignored while a sync runs) |
| `↑`/`k`, `↓`/`j` | move |

Switching view hides any revealed value.

## 1 · Vault

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
| `r` | reveal the value for 10 seconds |
| `y` | copy to the clipboard with `pass show -c`; pass clears it after 45 seconds |
| `n` | new pass entry: type a name, `enter`, type the value (masked), `enter`. It is unmanaged: not added to the mapping |
| `d` | delete the entry: type its exact name to confirm |
| `s` | start a sync and switch to the Sync view |
| `c` | re-read 1Password and refresh the list |

In any prompt, `esc` cancels and `backspace` deletes.

## 2 · Sync

Shows `running…` while a sync started with `s` runs, then a summary notice:
`synced: N changed · N unchanged · N skipped`.

A differing entry opens a conflict box with both lengths:

| Key | Action |
|---|---|
| `o` | overwrite pass with the 1Password value |
| `s` | skip this entry |
| `v` | show both values for 10 seconds |
| `q` | stop the sync |

There is no "overwrite all" key on purpose. The TUI sync has no prune or
skip-conflicts option; use `pvault sync --prune` or `--skip-conflicts`.

## 3 · History

Lists sync tags and store commits. `R` on a commit previews the rollback (entries
restored and removed) and runs it only after you type `rollback` and press `enter`.

## 4 · Mapping

Lists every `[[secret]]` with its reference, and any validation errors in red.

| Key | Action |
|---|---|
| `e` | open the mapping in `$EDITOR` (default `vi`); reloads and re-validates on exit |
| `a` | add from 1Password (below) |

Add from 1Password:

1. Pick an item (`↑`/`↓`, `enter`). The list comes from `op item list`.
2. Pick a field. Labels and references only; values are never loaded.
3. Edit the pass name (defaults to the item title in lower-case-with-dashes), `enter`.
4. Type an env name (upper-cased for you) or leave it empty, `enter`.

The entry is appended with scope `global`. `esc` closes the picker. Run a sync to
copy the value.

## Known issues

- **History view is empty.** It never loads commits or tags, so `R` has nothing to
  act on. Use `pvault history` and `pvault rollback` until this is fixed.
- **Sync and check show no per-entry lines.** The Sync view shows only `running…`
  and the summary; `c` only refreshes the entry list and says `check complete`. Use
  `pvault sync` or `pvault check` to see each entry.
