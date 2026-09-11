---
schema: 1
id: chezmoi
kind: topic
title: Chezmoi workflow
summary: Initialize, review, apply, update, and recover managed dotfiles
---

# Chezmoi workflow

Start with `chezmoi init <source>` and answer template prompts. Reconfigure later with
`chezmoi init`; review pending changes with `chezmoi diff`, then use `chezmoi apply`. Pull source
changes and apply them with `chezmoi update`. `chezmoi status` reports target state.

Files marked create-only are initialized when absent and then owned locally. If a create-only local
stub is deleted, run `chezmoi apply` to recreate it from the managed source. Use `chezmoi add --
<path>` to copy an authored local change back into the source state.
