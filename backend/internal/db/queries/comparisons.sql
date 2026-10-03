-- name: InsertComparison :exec
INSERT INTO comparisons (id, created_at, project_path, prompt, profile)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertSide :exec
INSERT INTO comparison_sides (comparison_id, side, config, cli_version, status, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: SaveSide :exec
UPDATE comparison_sides
SET status         = $3,
    end_reason     = $4,
    run_started_at = $5,
    ended_at       = $6,
    updated_at     = $7,
    phases         = $8,
    price_snapshot = $9,
    container_id   = $10,
    proxy          = $11,
    logs           = $12,
    token          = $13,
    failure        = $14,
    result         = $15,
    inputs         = $16,
    terminal       = coalesce(sqlc.narg('terminal'), terminal)
WHERE comparison_id = $1
  AND side = $2;

-- name: SaveReport :exec
UPDATE comparisons
SET report_status = $2,
    report        = $3
WHERE id = $1;

-- name: MarkCleaned :exec
UPDATE comparisons
SET cleaned_at = $2
WHERE id = $1;

-- name: DeleteComparison :exec
DELETE FROM comparisons
WHERE id = $1;

-- name: ListComparisons :many
SELECT *
FROM comparisons
ORDER BY created_at DESC;

-- name: ListSides :many
SELECT *
FROM comparison_sides;
