-- name: ListOrganizations :many
SELECT * FROM organizations ORDER BY parent_id NULLS FIRST, name;

-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = $1;

-- name: CreateOrganization :one
INSERT INTO organizations (name, parent_id, key)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteOrganization :exec
DELETE FROM organizations WHERE id = $1;
