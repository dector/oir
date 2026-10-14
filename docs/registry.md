# Registry aliases, search, and refresh

The official registry maps short names to existing GitHub tool specs. It is
published on this repository's `registry` branch as `index.kdl` and
`aliases.oini`.

## Commands

```sh
oir search oir       # Search alias names; shorthand: oir s oir
oir refresh          # Force an update of every registered registry
oir i oir            # Install using an alias
oir i oir@v1.2.3     # Override an alias's version
oir i -r oir         # Refresh registries before resolving and installing
oir i gh:dector/oir  # Explicit specs bypass alias lookup
```

Search includes user and registry aliases and is a case-insensitive substring
match on alias names. Results show the name, target spec, and source. User aliases override registry aliases; if
registries contain the same name, the first registered registry wins. Search
shows only the winning entry. Installation also reports whether resolution
used a user alias or a named registry. Unknown names produce an error suggesting
`oir refresh`.

Registry downloads are separate from user alias configuration. Search and
short-name installation fetch registries on first use and attempt an update
when their cache is older than 24 hours. If an update fails, they warn and use
the previous usable cache. Without a usable cache, a failed download is an error.
A missing name in a fresh cache does not trigger another download.

`oir refresh` attempts every registry, preserves each failed registry's previous
cache, and exits nonzero if any refresh fails. Successful updates remain saved.
`oir i --refresh` (or `-r`) uses the same strict refresh behavior and aborts
installation if any registry fails, even if an older cache exists. Both commands
report each failed registry and a final failure summary.

## Registry files

`index.kdl`:

```kdl
version "1"
aliases file="aliases.oini" type="oir-aliases"
```

`aliases.oini`:

```text
# Short names for GitHub tools
oir = gh:dector/oir
ror = gh:dector/ror
```

Oini supports `name = value`, blank lines, and full-line `#` comments. It does
not support INI sections or `;` comments. Registry alias names and target specs
are validated before a cache update is accepted.

The manifest must contain exactly one alias entry referencing a flat sibling
filename, not a path or URL. Both downloaded files must validate before the
cached snapshot is replaced. Unsupported schema versions or alias-file types
require an oir upgrade and never replace a working cache.

Only the official registry is registered in v1. Adding and removing registry
sources through the CLI is not supported.
