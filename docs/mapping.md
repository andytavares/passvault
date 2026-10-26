# The pvault mapping file

The mapping file lists which 1Password fields pvault copies into pass. It lives at
`~/.config/pvault/mapping.toml` (or `$XDG_CONFIG_HOME/pvault/mapping.toml` when
`XDG_CONFIG_HOME` is set). Create one with `pvault mapping init`, then add entries
in any of three ways:

- edit the file by hand;
- `pvault mapping add --pass … --ref … [--env …] [--scope …] [--command …]`, which
  validates before appending (see [commands.md](commands.md#mapping));
- in the TUI, open the Mapping tab and press `n` to pick an item and field from 1Password (see
  [tui.md](tui.md#mapping)).

Check it any time with:

```
pvault mapping validate
```

## Fields

Top level:

- `version` (required): file format version. Currently `1`. If a file has a newer
  version than your pvault supports, run `pvault upgrade`.
- `op_account` (optional): the 1Password account to read from, for example
  `my.1password.com`. Set it only when you have more than one account.

Each `[[secret]]` table:

- `pass` (required): path in the pass store, without the `.gpg` suffix. No leading
  `/` and no `..` segments. Must be unique in the file.
- `ref` (required): the 1Password secret reference, starting with `op://`.
- `env` (optional): environment variable name. Must match `[A-Z_][A-Z0-9_]*` and be
  unique in the file.
- `scope` (optional): `global` (the default), `command` or `none`. See below.
- `commands` (only with `scope = "command"`): the commands that get the variable.
  Names must match `[A-Za-z0-9_.-]+` because each becomes a shell function name.

## Getting a reference

The fastest way: in the 1Password app, open the item, open the field's menu and
choose **Copy Secret Reference**.

From the CLI:

```
op item get <item> --format json
```

and read `.fields[].reference` for the field you want.

References use the `op://vault/item/[section/]field` syntax. Vault, item, section
and field names may contain only letters, digits, `-`, `_`, `.` and spaces. For any
other character, use the item's ID instead of its name
(<https://www.1password.dev/cli/secret-reference-syntax/>).

## Scopes

- `global` (default): pvault exports the variable in every shell. Use it for tokens
  that many tools need, such as `GITHUB_TOKEN` when several CLIs read it.
- `command`: pvault sets the variable only for the listed commands, through a shell
  wrapper function per command. Requires `env` and `commands`. Use it for secrets
  you want to keep out of every other process's environment.
- `none`: pvault syncs the secret into pass but never puts it in the environment.
  Use it for values you read with `pass show` yourself.

## Example

```toml
version = 1
# op_account = "my.1password.com"   # only if you have more than one account

[[secret]]
pass = "pagerduty-api-key"                     # path in ~/.password-store, no .gpg
ref  = "op://Private/PagerDuty/credential"      # Copy Secret Reference in 1Password
env  = "PAGERDUTY_API_KEY"                      # optional
# scope = "global"                              # global (default) | command | none

[[secret]]
pass     = "work/github-token"
ref      = "op://Employee/GitHub PAT/token"
env      = "GITHUB_TOKEN"
scope    = "command"
commands = ["gh"]                               # set GITHUB_TOKEN only when gh runs
```

## Moving to another Mac

The file is per machine and holds no secrets, only references. Copy
`~/.config/pvault/mapping.toml` to the same path on the other Mac (respecting
`XDG_CONFIG_HOME` on both), then run `pvault mapping validate` and `pvault sync`.
