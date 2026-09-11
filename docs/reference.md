# CLI reference

`wtf` opens an fzf picker over installed/user-scoped commands, annotations, topics, and local tldr
matches represented in the merged inventory. `wtf <query>` displays a unique match, opens a
prefiltered picker for ambiguity, or resolves an exact PATH command. An explicit executable path
documents that file directly.

`wtf list` prints a terminal table or four tab-separated fields (`kind`, `id`, `availability`,
`summary`) when redirected. `-a` includes unavailable annotated commands. `--json` emits a sorted
array of records whose field names are versioned by the CLI release.

`wtf topic new <title>` slugifies the title, creates `<write_dir>/<id>.md`, and opens `$EDITOR`.
`wtf topic edit [query]` resolves by id/title or opens a topic-only picker. Invalid saves remain on
disk for recovery. Hooks run only after a changed valid save.

`wtf cache status [--json]` reports age, fingerprint state, staleness, and provider errors. `wtf
cache refresh` requests all providers to rebuild.

`wtf init zsh` prints a wrapper that checks active functions before aliases, then delegates to the
binary. Metadata is passed as quoted arguments. Direct binary use remains supported and says live
shell resolution is unavailable. Simple aliases may use the resolved executable's local package,
tldr, and native docs. Cycles and expansions containing shell operators are displayed without
guessing a target.

`wtf completion zsh`, `bash`, or `fish` prints a basic completion definition. There is deliberately
no `how` command in this release.
