-- name: ListFrameworks :many
SELECT * FROM frameworks ORDER BY name;

-- name: GetFramework :one
SELECT * FROM frameworks WHERE id = $1;

-- name: CreateFramework :one
INSERT INTO frameworks (name, short_name, version, description, framework_type)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateFramework :one
UPDATE frameworks
SET name = $2, short_name = $3, version = $4, description = $5, framework_type = $6,
    not_relevant = $7, not_relevant_reason = $8, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteFramework :exec
DELETE FROM frameworks WHERE id = $1;

-- name: ListRequirementsByFramework :many
SELECT *
FROM requirements
WHERE framework_id = $1
ORDER BY sort_order, ref;

-- name: GetRequirement :one
SELECT *
FROM requirements
WHERE id = $1;

-- name: CreateRequirement :one
INSERT INTO requirements (framework_id, ref, title, description, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateRequirement :one
UPDATE requirements
SET ref = $2, title = $3, description = $4, sort_order = $5,
    not_relevant = $6, not_relevant_reason = $7, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteRequirement :exec
DELETE FROM requirements WHERE id = $1;

-- name: CountRequirementsByFramework :one
SELECT COUNT(*)::bigint FROM requirements WHERE framework_id = $1 AND NOT not_relevant;

-- name: CountCoveredRequirementsByFramework :one
SELECT COUNT(DISTINCT r.id)::bigint
FROM requirements r
INNER JOIN measure_requirements mr ON r.id = mr.requirement_id
INNER JOIN measures m ON mr.measure_id = m.id
WHERE r.framework_id = $1
  AND m.status = 'implemented'
  AND NOT r.not_relevant;

-- name: ListMeasuresForRequirement :many
SELECT m.id, m.name, m.category, m.status, m.owner
FROM measures m
INNER JOIN measure_requirements mr ON m.id = mr.measure_id
WHERE mr.requirement_id = $1
ORDER BY m.name;
