-- name: GetPokemon :one
SELECT *
FROM pokemon
WHERE id = $1;
