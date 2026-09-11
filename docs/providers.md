# Providers and cache

`wtf` caches package-to-command mappings in `$XDG_CACHE_HOME/wtf/packages-v1.json`. Each provider
uses local installation metadata as a cheap fingerprint and refreshes on a fingerprint change or
after 24 hours. Writes use a same-directory temporary file plus atomic rename. A failed refresh
retains older records and marks them stale; a corrupt cache is discarded.

| Provider | Inventory and fingerprint |
| --- | --- |
| Homebrew | `brew leaves`, formula prefixes and their `bin` entries; Cellar directory metadata |
| mise | installed listing; mise installs directory metadata |
| uv | tool listing and entry points; uv tools directory metadata |
| Cargo | `.crates.toml` packages/binaries; manifest and bin directory metadata |
| Go | executable embedded module build info; Go bin directory metadata |
| APT/dpkg | exact command ownership only; dpkg status and APT extended-state metadata |
| Pacman | exact command ownership only; pacman local database metadata |
| PATH | exact executable lookup and shadow checking; PATH directory metadata |

Directory fingerprints intentionally use shallow metadata. A 24-hour TTL covers changes that keep
directory metadata stable. Homebrew casks, dependency-only libraries, and desktop applications do
not seed the picker. APT and Pacman do not seed their full manual package set: their metadata cannot
prove personal intent reliably. Annotated available commands appear, and any exact executable query
works immediately through PATH. Exact APT results exclude essential and required/important base
packages; exact Pacman ownership is reported without treating the system package set as a picker
seed.

Successful cheat.sh pages are cached for seven days. Requests happen only after selection and only
for commands backed by a known package or local tldr page. Topics, aliases, functions, and
annotation-only names are never sent. Offline failures preserve local output and use labeled stale
cache content when available.
