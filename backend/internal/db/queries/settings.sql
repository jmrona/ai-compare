-- name: GetSettings :one
SELECT data
FROM settings
WHERE id = 1;

-- name: SaveSettings :exec
INSERT INTO settings (id, data)
VALUES (1, $1)
ON CONFLICT (id) DO UPDATE SET data = excluded.data;
