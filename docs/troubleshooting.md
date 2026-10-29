# Troubleshooting

Start with `pvault doctor`. It checks most of what can go wrong.

| Message | Cause | Fix |
|---|---|---|
| `no mapping file at …; run pvault mapping init` | no mapping yet | `pvault mapping init`, then add entries |
| `op whoami: … Open 1Password > Settings > Developer > Integrate with 1Password CLI, then retry` | op cannot reach the app | turn on that setting, unlock 1Password, retry. With several accounts, set `op_account` in the mapping |
| `✕ NAME  failed: op read op://…: …` | the reference is wrong or you lack access | copy the reference again from 1Password (**Copy Secret Reference**). Names with characters other than letters, digits, `-`, `_`, `.` and spaces need the item ID instead |
| `pass store has uncommitted changes; commit or discard them (pass git status)` | someone edited the store outside pass's git | `pass git status`, then `pass git commit -am "…"` or `pass git checkout -- .` |
| `store is dirty; commit or discard changes first` | same, during rollback | same |
| `store was encrypted to a different key at REV` | the store was re-encrypted (`pass init` with a new key) after `REV` | roll back only to revisions after the key change |
| `version N is newer than this pvault supports, run pvault upgrade` | the mapping came from a newer pvault | `pvault upgrade` |
| `unknown keys: …` | a typo in the mapping | fix the key name; see [mapping.md](mapping.md#fields) |
| `pvault needs a GPG key` | you answered no to key creation | run `pvault setup` again and answer `y`, or create a key yourself and rerun setup |
| `found begin marker without end marker` | a managed block in `~/.zshrc` or `gpg-agent.conf` was half deleted | remove the leftover `# >>> pvault >>>` or `# <<< pvault <<<` line, rerun `pvault setup` |
| `vault deletion aborted: … does not match …` | the typed path for `--purge-vault` was wrong | type the exact store path shown, full or `~/…` |

## Symptoms

**A variable is missing in a new shell.**
Check, in order: `pvault status` shows the entry as `mapped` with a sync time; the
entry has `env` and scope `global` (command-scope values exist only inside the
wrapped command); `pvault env --zsh` prints the export, and its stderr has no
`pvault: skipped` line; `~/.zshrc` still has the managed block.

**A new shell asks for my GPG passphrase.**
Global values are decrypted at shell start. gpg-agent caches the passphrase after
the first prompt. Fewer global entries means fewer decryptions.

**Shell start is slow.**
Each global entry runs one `pass show`. Move rarely used ones to `scope = "command"`
or `"none"`.

**`pvault sync` in a script quits at the first conflict.**
With no input, the conflict prompt reads end-of-file and quits. Use
`--skip-conflicts`.

**`doctor` warns `zshrc … runs pass outside the managed block`.**
Those lines read pass directly, outside the mapping. Move them into the mapping so
pvault manages them, or ignore the warning if intended.

**My GPG key expired.**
Keys created by setup last two years. Extend it: `gpg --quick-set-expire <fingerprint> 2y`.

