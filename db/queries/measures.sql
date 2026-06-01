-- name: FilterMeasures :many
SELECT * FROM measures
WHERE
    (sqlc.arg(status) = '' OR status = sqlc.arg(status))
    AND (sqlc.arg(owner) = '' OR owner ILIKE '%' || sqlc.arg(owner) || '%')
    AND (NOT sqlc.arg(mine)::boolean OR assignee_id = sqlc.arg(assignee_id)::uuid)
ORDER BY name
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ListMeasures :many
SELECT * FROM measures ORDER BY name;

-- name: ListMeasuresForUser :many
SELECT * FROM measures WHERE assignee_id = $1 ORDER BY name LIMIT 10;

-- name: ListOwnedMeasures :many
SELECT
    id,
    name,
    status,
    to_char(updated_at, 'YYYY-MM-DD HH24:MI') AS updated_at
FROM measures
WHERE assignee_id = $1
ORDER BY updated_at DESC
LIMIT 25;

-- name: GetMeasure :one
SELECT * FROM measures WHERE id = $1;

-- name: CreateMeasure :one
INSERT INTO measures (name, description, category, owner, assignee_id, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateMeasure :one
UPDATE measures
SET name = $2, description = $3, category = $4, owner = $5, assignee_id = $6, status = $7, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteMeasure :exec
DELETE FROM measures WHERE id = $1;

-- name: LinkMeasureToRequirement :exec
INSERT INTO measure_requirements (measure_id, requirement_id, note)
VALUES ($1, $2, $3)
ON CONFLICT (measure_id, requirement_id) DO NOTHING;

-- name: UnlinkMeasureFromRequirement :exec
DELETE FROM measure_requirements
WHERE measure_id = $1 AND requirement_id = $2;

-- name: ListRequirementsForMeasure :many
SELECT
    r.id,
    r.framework_id,
    r.ref,
    r.title,
    r.description,
    r.sort_order,
    r.created_at,
    r.updated_at,
    f.name        AS framework_name,
    f.short_name  AS framework_short_name
FROM requirements r
INNER JOIN frameworks f ON r.framework_id = f.id
INNER JOIN measure_requirements mr ON r.id = mr.requirement_id
WHERE mr.measure_id = $1
ORDER BY f.name, r.sort_order, r.ref;

-- name: ListMeasureFrameworkLinks :many
-- Returns one row per (measure, framework) pair — used to build the framework
-- badge list shown next to each measure in the list view.
SELECT DISTINCT m.id AS measure_id, f.short_name AS framework_short_name
FROM measures m
INNER JOIN measure_requirements mr ON m.id = mr.measure_id
INNER JOIN requirements r ON mr.requirement_id = r.id
INNER JOIN frameworks f ON r.framework_id = f.id
ORDER BY m.id, f.short_name;

-- name: ListMeasureLinks :many
SELECT * FROM measure_links WHERE measure_id = $1 ORDER BY created_at;

-- name: AddMeasureLink :one
INSERT INTO measure_links (measure_id, url, label)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteMeasureLink :exec
DELETE FROM measure_links WHERE id = $1 AND measure_id = $2;
