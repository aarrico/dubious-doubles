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


def check_catalog(sql):
    """Exercise catalog constraints in one transaction, including rejected writes."""
    statements = ["""
        BEGIN;
        CREATE FUNCTION pg_temp.check_catalog(ok boolean, label text) RETURNS void
        LANGUAGE plpgsql AS $$
        BEGIN
            IF ok IS DISTINCT FROM TRUE THEN
                RAISE EXCEPTION 'Catalog assertion failed: %', label;
            END IF;
        END; $$;
        CREATE FUNCTION pg_temp.reject_catalog(statement text, expected_state text,
                                               expected_constraint text DEFAULT NULL)
        RETURNS void LANGUAGE plpgsql AS $$
        DECLARE actual_constraint text;
        BEGIN
            BEGIN
                EXECUTE statement;
            EXCEPTION WHEN OTHERS THEN
                GET STACKED DIAGNOSTICS actual_constraint = CONSTRAINT_NAME;
                IF SQLSTATE <> expected_state OR
                   (expected_constraint IS NOT NULL AND
                    actual_constraint <> expected_constraint) THEN
                    RAISE;
                END IF;
                RETURN;
            END;
            RAISE EXCEPTION 'Expected SQLSTATE % for: %', expected_state, statement;
        END; $$;
    """]

    def literal(value):
        if value is None:
            return "NULL"
        if isinstance(value, bool):
            return "TRUE" if value else "FALSE"
        if isinstance(value, int):
            return str(value)
        return "'" + value.replace("'", "''") + "'"

    def insert(table, **values):
        return (f"INSERT INTO {table} ({', '.join(values)}) VALUES "
                f"({', '.join(literal(value) for value in values.values())})")

    def accept(statement):
        statements.append(statement + ";")

    def reject(statement, state, constraint=None):
        accept("SELECT pg_temp.reject_catalog(" + ", ".join(
            literal(value) for value in (statement, state, constraint)) + ")")

    def check(query, label):
        accept(f"SELECT pg_temp.check_catalog(({query}), {literal(label)})")

    snapshots = dict(source_package="schema-probe", source_version="test",
                     game_set="test", locale="en", lockfile_sha256="a" * 64)
    for identity, digest in ((-1, "b"), (-2, "c")):
        accept(insert("catalog_snapshot", **dict(snapshots, id=identity,
                                                manifest_sha256=digest * 64)))
    check("SELECT count(*) = 2 AND bool_and(imported_at IS NOT NULL) "
          "FROM catalog_snapshot", "snapshot import timestamps")
    for field in ("lockfile_sha256", "manifest_sha256"):
        for value in ("A" * 64, "g" * 64, "a" * 63, "a" * 65, ""):
            reject(f"UPDATE catalog_snapshot SET {field} = {literal(value)} WHERE id = -1",
                   "23514")
    reject("UPDATE catalog_snapshot SET manifest_sha256 = repeat('b', 64) WHERE id = -2",
           "23505")
    for field in (*snapshots, "manifest_sha256"):
        reject(f"UPDATE catalog_snapshot SET {field} = NULL WHERE id = -1", "23502")

    pokemon = dict(catalog_snapshot_id=-1, name="schema-probe", type1="NORMAL",
                   base_hp=1, base_atk=1, base_def=1, base_spatk=1, base_spdef=1,
                   base_speed=1, dex_number=1, champions_id="shared-id", is_default=True,
                   is_battle_only_form=False, is_cosmetic_form=False, learnset_known=True)
    for identity, fields in (
        (-11, dict(slug="shared", learnset_known=False, height=100, weight=46000)),
        (-12, dict(slug="other", type2="FIRE", weight=94290)),
        (-13, dict(slug="form", form_name="Test form", base_species_id=-11,
                   is_default=False, is_battle_only_form=True)),
        (-14, dict(slug="empty-learnset", champions_id=None, is_default=False,
                   is_cosmetic_form=True)),
        (-21, dict(slug="shared", catalog_snapshot_id=-2)),
    ):
        accept(insert("pokemon", **dict(pokemon, id=identity, **fields)))

    options = {
        "ability": dict(name="schema-probe", effect="test"),
        "move": dict(name="schema-probe", pp=1, priority=0, damage_class="STATUS"),
        "item": dict(name="schema-probe", category="test", effect="test"),
    }
    for table, fields in options.items():
        for identity, snapshot in ((-11, -1), (-21, -2)):
            accept(insert(table, **dict(fields, id=identity, catalog_snapshot_id=snapshot,
                                        slug="shared")))

    for table in ("pokemon", *options):
        # Both snapshots may reuse a slug; neither may contain it twice.
        reject(f"UPDATE {table} SET catalog_snapshot_id = -1 WHERE id = -21", "23505")
        reject(f"UPDATE {table} SET slug = '' WHERE id = -11", "23514")
        reject(f"UPDATE {table} SET slug = NULL WHERE id = -11", "23502")
        reject(f"UPDATE {table} SET catalog_snapshot_id = NULL WHERE id = -11", "23502")
        reject(f"UPDATE {table} SET catalog_snapshot_id = -999 WHERE id = -21", "23503")

    for field in ("base_hp", "base_atk", "base_def", "base_spatk", "base_spdef",
                  "base_speed", "dex_number", "height", "weight"):
        for value in (0, -1):
            reject(f"UPDATE pokemon SET {field} = {value} WHERE id = -11", "23514")
        maximum = 2147483647 if field in ("dex_number", "weight") else 32767
        reject(f"UPDATE pokemon SET {field} = {maximum + 1} WHERE id = -11", "22003")
        if field not in ("height", "weight"):
            reject(f"UPDATE pokemon SET {field} = NULL WHERE id = -11", "23502")
    for field in ("type1", "type2"):
        reject(f"UPDATE pokemon SET {field} = 'INVALID' WHERE id = -11", "22P02")
    reject("UPDATE pokemon SET type1 = NULL WHERE id = -11", "23502")
    reject("UPDATE pokemon SET type2 = type1 WHERE id = -11", "23514")
    for field in ("is_default", "is_battle_only_form", "is_cosmetic_form", "learnset_known"):
        reject(f"UPDATE pokemon SET {field} = NULL WHERE id = -11", "23502")
    check("SELECT count(*) = 4 AND bool_and(column_default IS NULL) "
          "FROM information_schema.columns WHERE table_schema = 'public' "
          "AND table_name = 'pokemon' AND column_name IN "
          "('is_default', 'is_battle_only_form', 'is_cosmetic_form', 'learnset_known')",
          "form and learnset flags require explicit values")
    check("SELECT NOT EXISTS (SELECT FROM information_schema.columns "
          "WHERE table_schema = 'public' AND ((table_name = 'pokemon' AND column_name = 'order') "
          "OR (table_name = 'pokemon_ability' AND column_name = 'generation')))",
          "retired columns removed")

    def form(source, target, snapshot=-1):
        return insert("pokemon_form_relationship", catalog_snapshot_id=snapshot,
                      from_pokemon_id=source, to_pokemon_id=target)

    accept(form(-11, -13))
    accept(form(-12, -13))
    reject(form(-11, -13), "23505")
    reject(form(-11, -11), "23514")
    for source, target in ((-21, -13), (-11, -21), (-999, -13), (-11, -999)):
        reject(form(source, target), "23503")
    for base in (-21, -999):
        reject(f"UPDATE pokemon SET base_species_id = {base} WHERE id = -13", "23503")
    check("SELECT base_species_id = -11 AND (SELECT count(*) = 2 "
          "FROM pokemon_form_relationship WHERE to_pokemon_id = -13) "
          "FROM pokemon WHERE id = -13", "base species and multiple form predecessors")

    def ability_link(pokemon_id=-11, ability_id=-11, slot=1, identity=-201):
        return insert("pokemon_ability", id=identity, catalog_snapshot_id=-1,
                      pokemon_id=pokemon_id, ability_id=ability_id, slot=slot)

    for slot in (1, 2, 3):
        accept(ability_link(slot=slot, identity=-100 - slot))
    # Use a different identity to ensure the slot constraint, not the PK, rejects this.
    reject("INSERT INTO pokemon_ability (id, catalog_snapshot_id, pokemon_id, ability_id, slot) "
           "VALUES (-200, -1, -11, -11, 1)", "23505", "pokemon_ability_slot_unique")
    for slot in (0, -1, 4):
        reject(ability_link(pokemon_id=-12, slot=slot), "23514")
    reject("UPDATE pokemon_ability SET slot = NULL WHERE id = -101", "23502")
    for pokemon_id, ability_id, constraint in (
        (-21, -11, "pokemon_ability_pokemon_fk"),
        (-999, -11, "pokemon_ability_pokemon_fk"),
        (-12, -21, "pokemon_ability_ability_fk"),
        (-12, -999, "pokemon_ability_ability_fk"),
    ):
        # Avoid the existing identity and slot while testing each composite FK.
        reject(ability_link(pokemon_id, ability_id), "23503", constraint)

    def move_link(pokemon_id, move_id, snapshot=-1):
        return insert("pokemon_move", catalog_snapshot_id=snapshot,
                      pokemon_id=pokemon_id, move_id=move_id)

    accept(move_link(-12, -11))
    reject(move_link(-12, -11), "23505")
    for pokemon_id, move_id in ((-21, -11), (-12, -21), (-999, -11), (-12, -999)):
        reject(move_link(pokemon_id, move_id), "23503")
    check("SELECT count(*) = 2 AND count(*) FILTER (WHERE learnset_known) = 1 "
          "FROM pokemon p WHERE id IN (-11, -14) AND NOT EXISTS "
          "(SELECT FROM pokemon_move pm WHERE pm.pokemon_id = p.id)",
          "unknown and known-empty learnsets remain distinguishable")
    for table in ("pokemon", "ability", "move"):
        reject(f"DELETE FROM {table} WHERE id = -11", "23503")
    reject("DELETE FROM catalog_snapshot WHERE id = -1", "23503")
    check("SELECT count(*) = 2 FROM catalog_snapshot", "referenced old snapshot retained")
    statements.append("ROLLBACK;")
    sql("\n".join(statements))
    for table in ("catalog_snapshot", "pokemon", *options, "pokemon_form_relationship",
                  "pokemon_move", "pokemon_ability"):
        assert sql(f"SELECT count(*) FROM {table}") == "0", table
    print("PASS: catalog snapshots, identities, forms, learnsets, constraints and isolation", flush=True)


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

            schema_objects = ("catalog_snapshot", "pokemon", "pokemon_form_relationship",
                              "pokemon_move", "ability", "pokemon_ability", "move", "ailment",
                              "move_category", "move_target", "item")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NOT NULL") == "t"
            compose("run", "--rm", "migrate", "down-to", "0")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NULL") == "t"
            for enum in ("elemental_type", "damage_class"):
                assert sql(f"SELECT to_regtype('public.{enum}') IS NULL") == "t"
            compose("run", "--rm", "migrate", "up")
            for table in schema_objects:
                assert sql(f"SELECT to_regclass('public.{table}') IS NOT NULL") == "t"
            print("PASS: catalog schema rollback and reapplication", flush=True)

            check_catalog(sql)

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
