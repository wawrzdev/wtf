# CLI reference

`wtf` opens an fzf picker over installed/user-scoped commands, annotations, topics, and local tldr
matches represented in the merged inventory. `wtf <query>` displays a unique match, opens a
prefiltered picker for ambiguity, or resolves an exact PATH command. An explicit executable path
documents that file directly.

`wtf list` in a terminal prints records grouped under Aliases, Functions, Commands, Groups and
Topics, each as a name and short summary cut to the terminal width. Members of a group or
parent are folded into its row. The picker shows the same rows; choosing one with members
opens a second picker of them, and Esc returns. Redirected output prints four tab-separated
fields per record, members included (`kind`, `id`, `availability`, `summary`); annotated
aliases and functions report availability `shell`, because they live in the shell rather than
on PATH. Records and the picker are ordered by kind (alias, func, command, group, topic), then
id. Records come from annotations, topics and package-manager installs; local tldr pages add
examples to those records but never add records of their own. By default lists, lookups and
the picker cover installed commands, shell definitions, groups and topics, and `wtf <name>`
also works for any other command on PATH. `-a`/`--all` (`wtf -a`, `wtf list -a`) adds every
other executable on PATH as a system command (`"system": true` in JSON), described by its local
tldr page where one exists. Commands that are not installed are never listed; looking one up
reports that it is not installed. Picker rows are cut to the window width. `--json` emits the same ordered array of
records, with `parent` and `members` where set, whose field names are versioned by the CLI
release.

`wtf topic new <title>` slugifies the title, creates `<write_dir>/<id>.md`, and opens `$EDITOR`.
`wtf topic edit [query]` resolves by id/title or opens a topic-only picker. Invalid saves remain on
disk for recovery. Hooks run only after a changed valid save.

`wtf cache status [--json]` reports age, fingerprint state, staleness, and provider errors. `wtf
cache refresh` requests all providers to rebuild.

`wtf init zsh` prints a wrapper that checks active functions before aliases, then delegates to the
binary. Metadata is passed as quoted arguments. Direct binary use remains supported and says live
shell resolution is unavailable. Simple alias chains, up to 32 hops, may use the resolved
executable's local package, tldr, and native docs. Cycles, overlong chains, and expansions containing
shell operators or substitutions are displayed without guessing a target.

`wtf completion zsh`, `bash`, or `fish` prints contextual completions for commands, subcommands,
options, and locally known command/topic IDs. There is deliberately
no `how` command in this release.
