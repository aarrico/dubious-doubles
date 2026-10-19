# Dubious Doubles

*Questionable teams, explained.*

A competitive Pokémon team builder and learning companion focused on **Pokémon Champions doubles**. The goal is to help players put a team together, understand its tradeoffs, and feel ready to try it in battle.

Why do these teammates work together? Which weaknesses do they share? What does a move, ability, or item actually contribute? Dubious Doubles is being built around answering those questions with clear reasoning, visible assumptions, and room to experiment.

This is a personal project combining an interest in competitive Pokémon with building a practical Go backend.

## Status

**Early development.** The local backend foundation is runnable: a Go HTTP service, PostgreSQL, SQL migrations, health endpoints, and automated checks. Catalog lookup, saved teams, team analysis, and the frontend are planned features.

The API currently exposes operational health endpoints only. There is no playable catalog or team-building interface yet.

## Planned experience

- **Explore the catalog.** Find supported Pokémon and forms, then inspect their available moves, abilities, items, and stat choices.
- **Build and revise teams.** Save incomplete drafts, configure members, and edit choices as ideas develop.
- **Check readiness.** Understand verified rule violations, missing choices, and facts that still need verification.
- **Understand the team.** Explore attacking and defensive type coverage, shared weaknesses, and observations about Protect and speed control.

The first release targets a pinned Champions Ranked doubles rules context. Catalog facts and mechanics need Champions-specific evidence; unknowns should stay visible in the results.

Longer-term ideas include building around a favorite Pokémon, damage and stat calculations, matchup preparation, and learning from actual matches.

## Stack

| Component | Technology |
| --- | --- |
| API | Go, standard library HTTP routing, structured JSON logging |
| Database | PostgreSQL with pgx connection pooling |
| Migrations | Goose SQL migrations |
| Local environment | Docker Compose with pinned images |
| Verification | Go formatting, vet, unit tests, and real PostgreSQL integration checks |
| CI | GitHub Actions |

sqlc is planned for catalog query generation when the first catalog schema is implemented. The backend currently runs as one service.

## Run locally

You'll need Docker with Compose v2 or newer, Make, and OpenSSL to generate a local database password. Python 3 is required for integration checks. Host Go is optional for running the app; local formatting and Git hooks require the version specified in [backend/go.mod](backend/go.mod).

```sh
git clone https://github.com/aarrico/dubious-doubles.git
cd dubious-doubles
cp .env.example .env
openssl rand -hex 24
```

Replace `POSTGRES_PASSWORD` in `.env` with the generated value, then start the stack:

```sh
make up
```

Compose starts PostgreSQL, applies migrations, and waits for the API to become ready. The API is available at `http://127.0.0.1:8080`; PostgreSQL is accessible only within the Compose network. Set `API_PORT` in `.env` to use a different local port.

### Check the API

```sh
curl --fail http://127.0.0.1:8080/healthz
# {"status":"ok"}

curl --fail http://127.0.0.1:8080/readyz
# {"status":"ready"}
```

Liveness checks whether the API is running. Readiness checks its database connection and returns HTTP 503 with `{"status":"unavailable"}` when PostgreSQL is unavailable.

### Common commands

Run these from the repository root:

| Command | Purpose |
| --- | --- |
| `make up` | Build and start the stack, waiting for readiness |
| `make down` | Stop the stack and keep database data |
| `make status` | Show migration status |
| `make verify` | Check Go formatting, vet, and unit tests in Docker |
| `make integration` | Exercise the API and PostgreSQL in an isolated test stack |
| `make fmt` | Format working Go files with gofmt |
| `make hooks` | Enable local Git hooks for this clone |
| `make hooks-check` | Run the staged-file checks manually |

Inspect service logs with `docker compose logs api migrate db`.

Database data persists in a named Docker volume across shutdowns and container recreation. To deliberately delete the local database and start over, run **`make reset`**. This is destructive; ordinary shutdown uses `make down`.

## Catalog ingestion scaffold

[PokéPC export tooling](tools/pokepc-export/README.md) prepares pinned Champions
Pokémon, move, ability, item, and battle-state catalogs plus shared type and nature
references using Node. A separate Go command verifies the local snapshot without
npm, network access, or database writes. Database loading and complete
field/form validation are not implemented yet; this tooling is not part of API
startup.

## Development and checks

```sh
make verify
make integration
```

The integration suite creates its own Compose project, credentials, loopback port, and disposable database volume. It checks migrations and rollback, failed startup, database outage and recovery, HTTP health, graceful shutdown, and persistence across container recreation. It needs no development `.env` and cleans up its test containers and volume afterward.

GitHub Actions runs `verify` and `integration` as separate jobs.

### Git hooks

```sh
make hooks
```

Install hooks once per clone. Pre-commit checks staged content for Go formatting and syntax, whitespace errors, conflict markers, and missing final newlines. It preserves partial staging and unstaged edits. Pre-push runs `make verify` against the working tree.

Use `make fmt` to apply Go formatting, then review and stage the changes you want. `.editorconfig` provides consistent editor defaults for indentation and whitespace.

## Project layout

```text
backend/
  cmd/api/          API entry point and server lifecycle
  cmd/import-catalog/ Offline snapshot verification (database import pending)
  internal/config/  Configuration and validation
  internal/httpapi/ HTTP health handlers
  internal/ingest/  Snapshot integrity checks and ingestion scaffold
  migrations/       SQL migrations
  scripts/          Backend verification script
tools/pokepc-export/ Pinned Node exporter, tests, and usage guide
scripts/            Compose integration checks
.githooks/          Local Git hooks
.github/workflows/  CI jobs
compose.yaml        Local services and persistent storage
```

See [backend/README.md](backend/README.md) for API configuration and migration details.

## Contributing

Bug reports, reproducible examples, and suggestions are welcome. For changes to game data or mechanics, include the source and supported Champions context so the behavior can be checked. Run `make verify` and `make integration` before submitting a pull request.
