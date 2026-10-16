# Dubious Doubles backend

The runnable foundation uses Go, PostgreSQL, pgx, and Goose. Catalog queries will use sqlc as that feature is implemented. See the [project README](../README.md) for the planned experience and current status. Catalog, team, and analysis endpoints are not implemented yet.

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

The migration image pins Goose v3.29.0. SQL migrations live in `migrations`; use sequential numbers and Goose `Up`/`Down` annotations. The baseline establishes migration history only. Domain tables begin with catalog implementation. Migrations are transactional by default. A failed migration exits nonzero and blocks initial API startup. Inspect logs, correct an unapplied migration, and rerun `make up`. A later failed manual migration does not automatically stop an already running API.

`make down` removes containers/network and retains data. The following is a **destructive reset** deleting this project's database volume:

```sh
make reset
```

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

`integration` uses a randomly named Compose project, generated credentials, a Docker-assigned loopback port, and a disposable volume. It needs no `.env` and leaves the development database untouched. Checks cover fresh migration history, status/reapplication/transactional rollback, invalid startup configuration, HTTP health, database outage/recovery, bounded readiness, SIGTERM, and persistence after database/API container recreation. Probe tables exist only in the disposable database. Cleanup removes its containers and volume on success or assertion failure.

GitHub Actions runs these commands in jobs named `verify` and `integration`. Local checks do not establish hosted results. Repository branch rules are configured separately in GitHub.

Images are pinned by digest in Dockerfile/Compose; pgx is pinned with module checksums. The initial sqlc selection is v1.31.1; generation and generated-code verification will accompany the catalog queries. Tool references: [Goose SQL annotations](https://pressly.github.io/goose/documentation/annotations/), [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool), [PostgreSQL container storage](https://hub.docker.com/_/postgres).
