# Dubious Doubles backend

The runnable foundation uses Go, PostgreSQL, pgx, and Goose. sqlc generates catalog models and query methods for pgx. See the [project README](../README.md) for the planned experience and current status. Catalog, team, and analysis endpoints are not implemented yet.

## Start locally

Requirements: Docker Engine with Compose v2 or newer and Make. Python 3 is required for integration checks. Host Go is optional; build and verification run in a pinned image.

From the repository root:

```sh
cp .env.example .env
openssl rand -hex 24
# Replace POSTGRES_PASSWORD in .env with the generated value.
make up
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
make status
```

`make up` builds images, waits for PostgreSQL, applies migrations, and waits for API readiness. Expect liveness `{"status":"ok"}` and readiness `{"status":"ready"}`. Only the API publishes a host port, on loopback. Change `API_PORT` in `.env` if 8080 is occupied. Keep `.env` private and untracked. Use URL-safe letters and digits in the password because Compose embeds it in the database URL.

PostgreSQL 18 uses the `dubious_doubles` database and user, storing data in the named `postgres-data` volume mounted at `/var/lib/postgresql` under the `dubious-doubles` Compose project. Container recreation preserves it. Changing `.env` does not change an existing database password; restore the original value or deliberately reset expendable data.

## Operation and migrations

```sh
docker compose logs api migrate db
docker compose run --rm migrate status
docker compose run --rm migrate up
make down
```

The migration image pins Goose v3.29.0. SQL migrations live in `migrations`; use sequential numbers and Goose `Up`/`Down` annotations. `00001_catalog.sql` creates the snapshot-owned reference catalog. It replaces the former no-op baseline and base-data migration; existing development databases must be reset before applying this rewritten history. This is a fresh-install schema, not an upgrade migration. Migrations are transactional by default. A failed migration exits nonzero and blocks initial API startup. Inspect logs, correct an unapplied migration, and rerun `make up`. A later failed manual migration does not automatically stop an already running API.

`make down` removes containers/network and retains data. The following is a **destructive reset** deleting this project's database volume:

```sh
make reset
```

## Generate database code

From `backend/`, run:

```sh
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

This requires Go (the module pins its version). `sqlc.yaml` reads the Goose migrations and named SQL queries in `queries/`, then generates Go models, enum types, and query methods in `internal/db/` using pgx/v5. No running database or applied migrations are required for generation.

The initial query is `GetPokemon`, which looks up a Pokémon by catalog snapshot ID and stable source slug. Add named queries to `queries/` and rerun generation as needed. Edit the SQL or configuration, not the generated Go files. Keep generated types inside persistence code rather than exposing them as API contracts.

## Catalog storage contract

The schema supports the catalog import; it does not implement database loading or establish regulation eligibility. The importer must retain the original export (including manifest, files and LICENSE), verify its hashes, and insert each new snapshot and its facts in one transaction. Hash the exact `manifest.json` bytes for `manifest_sha256`; normalize SHA-256 values to lowercase. Store its source package/version, game set, locale and lockfile hash on `catalog_snapshot`. A repeated manifest is the same snapshot, not an instruction to overwrite rows. Never update/delete a previous snapshot's facts on reimport; old snapshots must remain available to saved teams. Foreign keys protect references, but do not make facts immutable: the importer must enforce this append-only policy. Database loading remains unimplemented.

- Pokémon, moves, abilities and items have integer internal IDs and snapshot-scoped source `slug` values. The same slug may occur in different snapshots. All Pokémon option, species and form foreign keys enforce same-snapshot ownership. The existing ailment/category/target lookup tables are descriptive scaffolding, not sourced executable mechanics; their import contract remains undefined.
- Map PokéPC `id` to `slug`, `dexNum` to `dex_number`, and optional `championsId` to text `champions_id`. Neither dex numbers nor Champions IDs are unique: forms share them. Types map to the uppercase enum values; base stats map directly to their six columns. Sort by `dex_number, slug` until an explicit form display order is needed; there is no invented source `order` field.
- Copy English `name` and optional `formName`. Map `isDefault`, `isBattleOnlyForm` and `isCosmeticForm` independently, without treating them as mutually exclusive. These flags require explicit values; do not guess missing source facts.
- Resolve optional `baseSpecies` to `base_species_id` within the same snapshot; leave it NULL when absent. Load canonical species before their forms. This grouping is separate from `pokemon_form_relationship`: each record's `baseForms` entries yield edges from the referenced base form to that record. Multiple predecessors are supported. Resolve references explicitly and reject missing records, rather than dropping them. Generic `formMethods` do not establish Champions transformation prerequisites.
- Set `learnset_known` from the presence of a source `learnset` array. An explicit empty array is known-empty; omission is unknown, not empty. Populate `pokemon_move` only from supplied move IDs. Learnset inheritance is not implemented and must not be inferred from a species/form edge alone.
- Map `ability1` to slot 1, `ability2` to slot 2, and `abilityHidden` to slot 3; retain gaps. Snapshot ownership replaces generation as the option-availability context. Hidden status is encoded by slot 3, so no second boolean can contradict it.
- `height` uses centimeters and `weight` uses units of 10 grams (kilograms × 100). Validate source units and convert when needed. Weight is an `INTEGER`, allowing source examples such as Snorlax `46000` and Mega Metagross `94290`. Unknown measurements remain NULL; supplied measurements and all base stats must be positive, and a second type cannot duplicate the first.

The PokéPC `8.2.0` Champions examples were inspected locally on October 19, 2026: Charizard, Mega Charizard X, Alolan Raichu, Venusaur and its female form, Snorlax, and Mega Metagross. This confirms the identifier, form, learnset-presence and numeric examples above, not M-C eligibility or complete source coverage. See the [source investigation](../docs/research/champions-data-sources.md) for evidence limits. Move/ability/item field mapping, missing learnset resolution and Champions transformation requirements still need their feature contracts; the schema alone does not complete those features.

## Configuration and health

| API environment | Default | Behavior |
| --- | --- | --- |
| `DATABASE_URL` | Required | PostgreSQL URL with host/database; errors omit credential-bearing input |
| `HTTP_ADDR` | `:8080` | Host and numeric port inside the container |
| `DATABASE_TIMEOUT` | `2s` | Database operation/connection deadline; allowed range 1ms–5s |

The pool is capped at four connections. `GET /healthz` returns 200 without database access; `GET /readyz` pings PostgreSQL and returns 200 or a generic 503. Readiness recovers when the database returns. Responses use JSON and disable caching. Operational health routes sit outside `/api/v1`, reserved for later features.

HTTP headers, reads, writes, idle connections, and shutdown have finite limits in `cmd/api/main.go`. SIGINT/SIGTERM initiate graceful shutdown with a ten-second deadline; Compose allows fifteen seconds. Logs are JSON on stdout. API and migration containers run as a numeric non-root user. Hosting remains a future decision.

## Verify

From the repository root:

```sh
make verify
make integration
```

`verify` checks gofmt, vet, and unit tests in the pinned Go image. With Go 1.27.2 installed, run `cd backend && sh scripts/verify.sh` for equivalent local checks.

`integration` uses a randomly named Compose project, generated credentials, a Docker-assigned loopback port, and a disposable volume. It needs no `.env` and leaves the development database untouched. Checks cover fresh migration history, full schema rollback/reapplication, snapshot-scoped identity and references, form relationships, missing/empty learnsets, ability slots, numeric/type constraints, invalid startup configuration, HTTP health, database outage/recovery, bounded readiness, SIGTERM, and persistence after database/API container recreation. Probe tables exist only in the disposable database. Cleanup removes its containers and volume on success or assertion failure.

GitHub Actions runs these commands in jobs named `verify` and `integration`. Local checks do not establish hosted results. Repository branch rules are configured separately in GitHub.

Images are pinned by digest in Dockerfile/Compose; pgx is pinned with module checksums. sqlc generation is pinned to v1.31.1 in the command above. Tool references: [Goose SQL annotations](https://pressly.github.io/goose/documentation/annotations/), [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool), [PostgreSQL container storage](https://hub.docker.com/_/postgres).
