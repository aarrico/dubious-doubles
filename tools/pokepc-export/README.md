# PokéPC snapshot export

This is an offline ingestion tool in the main repository, not an API dependency
or a separate service. It resolves Pokémon, moves, abilities, items, battle states,
and their English text using the pinned `@pokepc/dataset` Champions loader. It also
captures the package's shared type and nature reference catalogs. The Go backend
consumes the output without Node, npm, or network access.

## Export and check

Requirements: Node 24+ and pnpm 12.6.0 for export; the backend's pinned Go version
for the check command. Installation needs access to the npm registry (or a populated
local pnpm store). Export itself reads installed files only.

From this directory:

```sh
pnpm install --frozen-lockfile --ignore-scripts
pnpm test
pnpm run export
```

The default output is `.cache/pokepc/8.2.0/format-2/` at the repository root.
This leaves any earlier Pokémon-only snapshot intact. From `backend/`:

```sh
go run ./cmd/import-catalog -check -snapshot ../.cache/pokepc/8.2.0/format-2
```

**This verifies a snapshot; it does not import database rows yet.** Omitting
`-check` fails explicitly. PostgreSQL and migrations are not needed for this check.

An optional export argument selects a different output directory, relative to
your current working directory:

```sh
pnpm run export ./another-snapshot
```

Existing output directories are never overwritten. For a second export, choose a
new path. If an export is interrupted, inspect and remove only that failed output
before retrying. `POKEPC_DATASET_DIR` overrides are rejected so an export cannot
silently use unrelated data under the pinned package's name.

## Snapshot format 2

Each catalog file contains `scope`, `records` (upstream objects sorted by ID), and
`text` (English entries keyed by ID). These files are independent of the eventual
database schema; all upstream record and text fields and nested array order are
preserved.

| File | Scope | Intended use |
| --- | --- | --- |
| `pokemon.json` | `champions` | Identity, types, base stats, abilities, learnsets, and form relationships |
| `moves.json` | `champions` | Move options, properties, descriptions, and `usable` flags |
| `abilities.json` | `champions` | Ability options, properties, and descriptions |
| `items.json` | `champions` | Item options, properties, descriptions, and held-item restrictions in the source |
| `battle-states.json` | `champions` | Source battle-state records and descriptions, including weather and terrain |
| `types.json` | `base` | Shared type IDs, display metadata, and English names |
| `natures.json` | `base` | Shared nature IDs, stat associations, and English names |

`LICENSE` retains the upstream MIT notice verbatim. `manifest.json` records format
version 2, the exact package version, game set, locale, tooling lockfile SHA-256,
and SHA-256 hashes of all seven catalog files and the license. The Go reader also
continues to accept the original format 1 Pokémon-only snapshot.

### Retained data is not a selectable-options or mechanics contract

No move, item, ability, or battle-state records are filtered out. In particular,
`usable: false` moves remain in the snapshot for provenance; export presence must
not make them selectable. The source roster is not an M-C eligibility list, and
item export does not establish that an item may be held under the selected rules.

Pokémon learnsets, ability IDs, and form relationships remain in `pokemon.json`;
they are not duplicated into separate relationship files. The exporter does not
invent missing learnsets or resolve inherited form relationships. Generic fields
in merged Pokémon records (such as other-game availability or transformation
methods) remain upstream reference information, not verified Champions facts.

Types and natures are explicitly `base` references because these catalogs have no
Champions mod. All source entries are retained (including the noncanonical
`stellar` type); this does not assert Champions availability. `types.json` supplies
no type-effectiveness chart. Nature data does not establish Champions allocation
limits or stat arithmetic. Those facts, regulation rules, and full transformation
prerequisites still need their own verified contracts. Descriptions and upstream
tags are not an executable battle engine or verified analysis classifications.

Export coverage was checked against the installed `@pokepc/dataset@8.2.0` package
on October 19, 2026. Tests compare exported data to the loader's complete merged
Champions catalogs and the two shared source catalogs.

Object keys are sorted and there are no timestamps or absolute paths in exported
content. Two exports from the same installed input and tooling should be byte
identical. The committed lockfile pins dependency integrity, including the loader's
transitive dependencies. Install with `--frozen-lockfile`; the exporter does not
independently authenticate or rehash every installed dependency. Snapshot hashes
check corruption relative to the manifest, not authenticity.

Generated snapshots and `node_modules` are ignored. Archive a successful snapshot
and its manifest when it becomes an approved import input; do not fetch fresh
upstream data during API startup. Retain referenced snapshots when database loading
is implemented. No committed full dataset or durable artifact store is set up yet.

## Remaining implementation

The Go scaffold checks hashes, supported manifest context and catalog scopes,
nonempty records, unique IDs within each catalog, and English names. It retains
raw JSON for subsequent mapping. It does not yet validate cross-catalog references,
learnset inheritance, or regulation eligibility.

Next, finish the [Pokémon source contract](../../docs/issues/04-rules-and-source-coverage.md):
inspect Charizard, a regional form, and a battle-only form; define expected IDs,
valid types/stats, and form-reference handling. Then implement the domain mapping,
complete validation, transactional/idempotent persistence, and source-snapshot
storage. A successful snapshot check is **not** a catalog completeness audit or
proof of M-C eligibility. Do not put data imports inside schema migrations.

To upgrade the source, explicitly change the exact dependency pin, refresh and
review the lockfile, rerun tests, export to a new directory, and review the input
changes. If the payload contract changes incompatibly, bump its format version
and update the Go reader together. Keep database mapping in Go, not this exporter.
