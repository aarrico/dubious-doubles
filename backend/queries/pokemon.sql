-- name: GetPokemon :one
SELECT *
FROM pokemon
WHERE catalog_snapshot_id = $1 AND slug = $2;
