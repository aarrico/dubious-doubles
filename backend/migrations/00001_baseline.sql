-- +goose Up
-- Establish migration history before the catalog schema is decided in M1.
SELECT 1;

-- +goose Down
SELECT 1;
