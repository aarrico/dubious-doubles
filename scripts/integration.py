#!/usr/bin/env python3
"""Exercise the real Compose stack in a disposable project and volume."""
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def main():
    project = "dubious-doubles-test-" + secrets.token_hex(5)
    password = secrets.token_hex(24)
    latest_version = max(int(path.name.split("_", 1)[0])
                         for path in (ROOT / "backend" / "migrations").glob("*.sql"))
    with tempfile.TemporaryDirectory(prefix=project) as temporary:
        directory = Path(temporary)
        env_file = directory / "compose.env"
        env_file.write_text(f"POSTGRES_PASSWORD={password}\nAPI_PORT=0\n")
        env_file.chmod(0o600)
        environment = os.environ.copy()
        # The test's credentials and port must win over the developer's shell.
        environment.update(POSTGRES_PASSWORD=password, API_PORT="0")
        base = ["docker", "compose", "--project-name", project,
                "--env-file", str(env_file), "-f", str(ROOT / "compose.yaml")]

        def compose(*arguments, success=True, timeout=120, override=None):
            command = base + (["-f", str(override)] if override else []) + list(arguments)
            result = subprocess.run(command, cwd=ROOT, env=environment,
                                    text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT, timeout=timeout)
            output = result.stdout.replace(password, "[redacted]")
            if success and result.returncode:
                raise AssertionError(f"Compose {arguments[0]} failed:\n{output[-10000:]}")
            if not success and result.returncode == 0:
                raise AssertionError("Expected Compose command to fail")
            return output

        def sql(statement, success=True):
            return compose("exec", "-T", "db", "psql", "-U", "dubious_doubles", "-d", "dubious_doubles",
                           "-v", "ON_ERROR_STOP=1", "-Atc", statement, success=success).strip()

        def address():
            return "http://" + compose("port", "api", "8080").strip()

        def expect(path, status, body):
            request = urllib.request.Request(address() + path)
            try:
                response = urllib.request.urlopen(request, timeout=6)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                assert response.code == status, (path, response.code, status)
                assert response.headers.get_content_type() == "application/json"
                assert json.load(response) == {"status": body}

        def wait_ready():
            deadline = time.monotonic() + 40
            while True:
                try:
                    expect("/readyz", 200, "ready")
                    return
                except (AssertionError, urllib.error.URLError, TimeoutError):
                    if time.monotonic() >= deadline:
                        raise
                    time.sleep(0.5)

        try:
            compose("build", timeout=900)
            broken = directory / f"{latest_version + 1:05d}_failure.sql"
            broken.write_text("-- +goose Up\nCREATE TABLE rollback_probe (id integer);\n"
                              "SELECT * FROM intentionally_missing_relation;\n"
                              "-- +goose Down\nDROP TABLE rollback_probe;\n")
            broken.chmod(0o644)
            override = directory / "failure.json"
            override.write_text(json.dumps({"services": {"migrate": {"volumes": [
                f"{broken}:/app/migrations/{broken.name}:ro"]}}}))
            compose("up", "--wait", "--wait-timeout", "120", override=override, success=False)
            assert "intentionally_missing_relation" in compose("logs", "migrate")
            assert not compose("ps", "--status", "running", "--quiet", "api").strip()
            assert sql("SELECT to_regclass('public.rollback_probe') IS NULL") == "t"
            assert sql("SELECT max(version_id) FROM goose_db_version WHERE is_applied") == str(latest_version)
            print("PASS: failed migration rolls back and blocks API startup", flush=True)

            compose("up", "--wait", "--wait-timeout", "120")
            expect("/healthz", 200, "ok")
            expect("/readyz", 200, "ready")
            assert sql("SELECT max(version_id) FROM goose_db_version WHERE is_applied") == str(latest_version)
            assert "Applied At" in compose("run", "--rm", "migrate", "status")
            compose("run", "--rm", "migrate", "up")
            assert sql("SELECT count(*) FROM goose_db_version WHERE version_id=1") == "1"
            assert sql(f"SELECT count(*) FROM goose_db_version WHERE version_id={latest_version}") == "1"
            print("PASS: fresh migrations, status, repeat up, HTTP health", flush=True)

            schema_objects = ("pokemon", "ability", "pokemon_ability", "move", "ailment",
                              "move_category", "move_target", "item")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NOT NULL") == "t"
            compose("run", "--rm", "migrate", "down-to", "1")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NULL") == "t"
            for enum in ("elemental_type", "damage_class"):
                assert sql(f"SELECT to_regtype('public.{enum}') IS NULL") == "t"
            compose("run", "--rm", "migrate", "up")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NOT NULL") == "t"
            print("PASS: catalog schema rollback and reapplication", flush=True)

            fixture = (
                "BEGIN; "
                "INSERT INTO pokemon (id, name, type1, base_hp, base_atk, base_def, "
                "base_spatk, base_spdef, base_speed, \"order\") "
                "VALUES (-1, 'schema-probe', 'NORMAL', 1, 1, 1, 1, 1, 1, 1); "
                "INSERT INTO ability (id, name, effect) "
                "VALUES (-1, 'schema-probe', 'test'); "
            )
            link = "INSERT INTO pokemon_ability (pokemon_id, ability_id, slot, generation) "
            sql(fixture + link + "VALUES (-1, -1, 1, NULL), (-1, -1, 2, NULL), "
                "(-1, -1, 1, 1), (-1, -1, 1, 2); ROLLBACK;")
            for values, constraint in (
                ("(-2, -1, 1, NULL)", "pokemon_ability_pokemon_fk"),
                ("(-1, -2, 1, NULL)", "pokemon_ability_ability_fk"),
                ("(-1, -1, 1, NULL), (-1, -1, 1, NULL)", "pokemon_ability_slot_unique"),
                ("(-1, -1, 1, 1), (-1, -1, 1, 1)", "pokemon_ability_slot_unique"),
            ):
                output = sql(fixture + link + "VALUES " + values + "; ROLLBACK;", success=False)
                assert constraint in output, output
            assert sql("SELECT count(*) FROM pokemon") == "0"
            assert sql("SELECT count(*) FROM ability") == "0"
            assert sql("SELECT count(*) FROM pokemon_ability") == "0"
            print("PASS: ability foreign keys and slot uniqueness, including NULL generation", flush=True)

            output = compose("run", "--rm", "--no-deps", "-e", "DATABASE_URL=invalid",
                             "api", success=False)
            assert "DATABASE_URL" in output
            print("PASS: invalid configuration fails at startup", flush=True)

            compose("stop", "db")
            started = time.monotonic()
            expect("/readyz", 503, "unavailable")
            assert time.monotonic() - started < 6
            expect("/healthz", 200, "ok")
            compose("start", "db")
            wait_ready()
            print("PASS: database outage, bounded readiness, liveness, recovery", flush=True)

            sql("CREATE TABLE persistence_probe (value text NOT NULL); "
                "INSERT INTO persistence_probe VALUES ('survives-recreation')")
            compose("stop", "api")
            container = compose("ps", "--all", "--quiet", "api").strip()
            inspected = subprocess.check_output(["docker", "inspect", container], text=True)
            assert json.loads(inspected)[0]["State"]["ExitCode"] == 0
            assert "API shutting down" in compose("logs", "api")
            compose("up", "--wait", "--force-recreate", "--no-deps", "db")
            compose("up", "--wait", "--force-recreate", "--no-deps", "api")
            wait_ready()
            assert sql("SELECT value FROM persistence_probe") == "survives-recreation"
            sql("DROP TABLE persistence_probe")
            print("PASS: graceful SIGTERM and persistence across container recreation", flush=True)
        finally:
            compose("down", "--volumes", "--remove-orphans")
    print("Integration checks passed; disposable containers and volume removed.", flush=True)


if __name__ == "__main__":
    main()
