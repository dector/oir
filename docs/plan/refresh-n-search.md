# Registry, refresh, and search — v1 plan

## Goal

Resolve short tool names through cached, GitHub-hosted registries, reusing the existing alias mechanics and installation pipeline.

```sh
oir i bar              # Resolve a user alias or registry alias
oir i gh:foo/bar       # Explicit spec bypasses alias lookup
oir i bar@v1.2.3       # Override the resolved alias's version
```

Registries provide aliases only in v1. They do not introduce a new installation backend.

## Registry configuration

- Support multiple registries internally from day one.
- Register only the official registry initially, in code.
- No user-facing commands to create, add, or remove registries in v1.
- Each registry has a stable source name and an index URL.
- Registry list order determines precedence. There is no numeric priority field.
- Host the official registry in this repository on the `registry` branch.
- Serve the registry from that branch's repository root: `index.kdl` and its sibling alias file live at the root, not in a subdirectory.

## Registry format

Each registry publishes an `index.kdl` manifest:

```kdl
version "1"
aliases file="aliases.oini" type="oir-aliases"
```

- `version` identifies the manifest schema version, not a tool version.
- V1 requires exactly one `aliases` entry. Missing or duplicate entries are errors.
- `file` must name a flat sibling file beside `index.kdl`.
- No directory paths, traversal, or external URLs are supported for `file` in v1.
- `oir-aliases` identifies oini, the existing alias-file syntax. Use `aliases.oini` as the registry filename.
- Oini supports `name = value`, blank lines, and full-line `#` comments. It does not support INI sections or `;` comments.

```text
bar = gh:foo/bar
rg = gh:BurntSushi/ripgrep
```

Unsupported manifest versions or alias-file types reject the refresh with a clear error explaining that an oir upgrade is required. Preserve any previous cache.

## Resolution and provenance

Resolution order:

1. Explicit tool specs bypass alias lookup.
2. User-defined aliases take precedence over all registry entries.
3. Check registries in registration order; the first matching entry wins.
4. If no entry matches, report that the tool was not found and suggest `oir refresh`.

A missing name in a fresh cache does not trigger another download.

Show the resolution source during installation, distinguishing user aliases and named registries. Keep provenance separate from the resolved tool spec so the existing installer can remain independent of alias lookup.

## Cache policy

Keep downloaded registry caches separate from user-owned alias configuration, with a separate snapshot per registry.

Normal installation and search use the same policy:

- Fetch a registry on first use if no cache exists.
- Attempt an automatic refresh when its cache is older than 24 hours.
- If refresh fails and an existing usable cache is available, warn and use that cache.
- If fetching fails and no usable cache exists, return an error.

A refresh is all-or-nothing per registry:

1. Download the manifest and its referenced alias file.
2. Validate both files.
3. Replace the cached snapshot only after the complete refresh succeeds.

Any failure preserves the previous snapshot. Do not mix a newly downloaded manifest with an old alias file.

## Explicit refresh command

```sh
oir refresh
```

- Force-refresh every registered registry, regardless of cache age.
- Continue attempting other registries after a failure.
- Keep successful updates; there is no cross-registry rollback.
- Preserve the previous cache for each failed registry.
- Print an error for each failed registry.
- Print a final error summary and exit nonzero if any registry failed.

## Refresh before installation

```sh
oir i --refresh bar
oir i -r bar
```

- Force-refresh registries before resolving and installing the requested tool.
- Use the same refresh machinery as `oir refresh`, including attempting all registries and reporting failures.
- If any registry refresh fails, return an error and abort installation.
- Do not fall back to stale caches: the user explicitly requested fresh registry data.
- Successful registry cache updates remain saved even if another registry fails.

## Search

```sh
oir search bar
oir s bar
```

Keep search minimal in v1:

- Case-insensitive substring matching on alias names.
- Search user aliases and cached registry aliases.
- Show each matching name, target spec, and source.
- Deduplicate names using installation precedence, showing only the winning entry.
- Use the normal cache policy described above.
- No fuzzy matching, ranking, descriptions, or advanced query syntax.

## Implementation direction

- Reuse existing alias parsing and validation rather than introducing a separate alias syntax.
- Separate short-name resolution from user-alias storage so it can consult both user aliases and registry caches.
- Return resolution provenance alongside the tool spec.
- Share cache and refresh logic between installation, search, and the refresh command.
- The existing `internal/registry` package describes installation backends. Keep this name-resolution catalog concept separate; `catalog` is a suitable internal package name.

## Validation checklist

- User aliases override registry entries.
- Registry list order breaks duplicate-name conflicts.
- Explicit specs retain their direct-resolution behavior.
- Alias version overrides still work.
- Installation reports the winning resolution source.
- Missing, fresh, and expired caches follow the specified policy.
- A missing name in a fresh cache does not force a network request.
- Invalid manifests, unsafe filenames, unsupported versions/types, and invalid alias files cannot replace a valid snapshot.
- Refresh failures leave existing caches intact.
- `oir refresh` continues after failures and returns an aggregate error.
- `oir i -r` aborts installation on any refresh failure, even with usable old caches.
- Search matches names case-insensitively and deduplicates consistently with installation.

## Official registry location

Use this repository's `registry` branch, with `index.kdl` at the repository root. Configure the official index URL to point to that file.
