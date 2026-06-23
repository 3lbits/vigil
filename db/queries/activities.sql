-- name: FilterActivities :many
SELECT
    a.id,
    a.measure_id,
    a.title,
    a.description,
    a.activity_type,
    a.recurrence,
    a.status,
    a.priority,
    a.kind,
    a.owner,
    a.assignee_id,
    a.due_date,
    a.completed_at,
    a.completed_by,
    a.notes,
    a.evidence_url,
    a.parent_activity_id,
    a.created_at,
    a.updated_at,
    a.ref_num,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
WHERE
    (sqlc.arg(status) = '' OR a.status = sqlc.arg(status))
    AND (sqlc.arg(kind) = '' OR a.kind = sqlc.arg(kind))
    AND (sqlc.arg(q) = '' OR a.title ILIKE '%' || sqlc.arg(q) || '%' OR a.owner ILIKE '%' || sqlc.arg(q) || '%' OR a.description ILIKE '%' || sqlc.arg(q) || '%')
    AND (NOT sqlc.arg(mine)::boolean OR a.assignee_id = sqlc.arg(assignee_id)::uuid)
ORDER BY
    CASE WHEN sqlc.arg(sort) = 'default' THEN
        CASE a.status
            WHEN 'overdue'     THEN 0
            WHEN 'in_progress' THEN 1
            WHEN 'planned'     THEN 2
            ELSE 3
        END
    END ASC,
    CASE WHEN sqlc.arg(sort) = 'default' THEN a.due_date END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort) = 'default' THEN a.created_at END DESC,
    CASE WHEN sqlc.arg(sort) = 'title' AND sqlc.arg(dir) = 'asc' THEN a.title END ASC,
    CASE WHEN sqlc.arg(sort) = 'title' AND sqlc.arg(dir) = 'desc' THEN a.title END DESC,
    CASE WHEN sqlc.arg(sort) = 'kind' AND sqlc.arg(dir) = 'asc' THEN a.kind END ASC,
    CASE WHEN sqlc.arg(sort) = 'kind' AND sqlc.arg(dir) = 'desc' THEN a.kind END DESC,
    CASE WHEN sqlc.arg(sort) = 'owner' AND sqlc.arg(dir) = 'asc' THEN a.owner END ASC,
    CASE WHEN sqlc.arg(sort) = 'owner' AND sqlc.arg(dir) = 'desc' THEN a.owner END DESC,
    CASE WHEN sqlc.arg(sort) = 'due_date' AND sqlc.arg(dir) = 'asc' THEN a.due_date END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort) = 'due_date' AND sqlc.arg(dir) = 'desc' THEN a.due_date END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort) = 'status' AND sqlc.arg(dir) = 'asc' THEN a.status END ASC,
    CASE WHEN sqlc.arg(sort) = 'status' AND sqlc.arg(dir) = 'desc' THEN a.status END DESC,
    CASE WHEN sqlc.arg(sort) = 'created_at' AND sqlc.arg(dir) = 'asc' THEN a.created_at END ASC,
    CASE WHEN sqlc.arg(sort) = 'created_at' AND sqlc.arg(dir) = 'desc' THEN a.created_at END DESC,
    a.id ASC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ListActivities :many
SELECT
    a.id,
    a.measure_id,
    a.title,
    a.description,
    a.activity_type,
    a.recurrence,
    a.status,
    a.priority,
    a.kind,
    a.owner,
    a.assignee_id,
    a.due_date,
    a.completed_at,
    a.completed_by,
    a.notes,
    a.evidence_url,
    a.parent_activity_id,
    a.created_at,
    a.updated_at,
    a.ref_num,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
ORDER BY
    CASE a.status
        WHEN 'overdue'     THEN 0
        WHEN 'in_progress' THEN 1
        WHEN 'planned'     THEN 2
        ELSE 3
    END,
    a.due_date ASC NULLS LAST,
    a.created_at DESC;

-- name: GetActivity :one
SELECT
    a.id,
    a.measure_id,
    a.title,
    a.description,
    a.activity_type,
    a.recurrence,
    a.status,
    a.priority,
    a.kind,
    a.owner,
    a.assignee_id,
    a.due_date,
    a.completed_at,
    a.completed_by,
    a.notes,
    a.evidence_url,
    a.parent_activity_id,
    a.created_at,
    a.updated_at,
    a.ref_num,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
WHERE a.id = $1;

-- name: CreateActivity :one
INSERT INTO activities (measure_id, title, description, activity_type, recurrence, priority, kind, owner, assignee_id, due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateActivity :one
UPDATE activities
SET title         = $2,
    description   = $3,
    activity_type = $4,
    recurrence    = $5,
    priority      = $6,
    kind          = $7,
    owner         = $8,
    assignee_id   = $9,
    due_date      = $10,
    status        = CASE
                        WHEN $10::date < CURRENT_DATE AND status IN ('planned', 'in_progress')
                            THEN 'overdue'
                        WHEN ($10 IS NULL OR $10::date >= CURRENT_DATE) AND status = 'overdue'
                            THEN 'planned'
                        ELSE status
                    END,
    updated_at    = NOW()
WHERE id = $1
RETURNING *;

-- name: CompleteActivity :one
UPDATE activities
SET status       = 'completed',
    completed_at = NOW(),
    completed_by = $2,
    notes        = $3,
    evidence_url = $4,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: ReopenActivity :one
UPDATE activities
SET status       = 'planned',
    completed_at = NULL,
    completed_by = '',
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: MarkActivityInProgress :one
UPDATE activities
SET status = 'in_progress', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: MarkOverdueActivities :exec
UPDATE activities
SET status = 'overdue', updated_at = NOW()
WHERE due_date < CURRENT_DATE
  AND status IN ('planned', 'in_progress');

-- name: DeleteActivity :exec
DELETE FROM activities WHERE id = $1;

-- name: ListRecentActivities :many
SELECT
    a.id,
    a.title,
    a.status,
    a.owner,
    a.priority,
    a.completed_at,
    a.due_date,
    a.updated_at,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
ORDER BY a.updated_at DESC
LIMIT 5;

-- name: ListActivitiesForUser :many
SELECT
    a.id,
    a.title,
    a.status,
    a.priority,
    a.kind,
    a.due_date,
    a.updated_at,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
WHERE a.assignee_id = $1
ORDER BY
    CASE a.status
        WHEN 'overdue'     THEN 0
        WHEN 'in_progress' THEN 1
        WHEN 'planned'     THEN 2
        ELSE 3
    END,
    a.due_date ASC NULLS LAST
LIMIT 10;

-- name: ListOwnedActivities :many
SELECT
    a.id,
    a.title,
    a.status,
    a.priority,
    a.kind,
    (COALESCE(to_char(a.due_date::date, 'YYYY-MM-DD'), ''::text))::text AS due_date,
    to_char(a.updated_at, 'YYYY-MM-DD HH24:MI') AS updated_at,
    COALESCE(m.name, '') AS measure_name
FROM activities a
LEFT JOIN measures m ON a.measure_id = m.id
WHERE a.assignee_id = $1
ORDER BY
    CASE a.status
        WHEN 'overdue'     THEN 0
        WHEN 'in_progress' THEN 1
        WHEN 'planned'     THEN 2
        ELSE 3
    END,
    a.due_date ASC NULLS LAST,
    a.updated_at DESC
LIMIT 25;

-- name: ListActivitiesForMeasure :many
SELECT
    a.id,
    a.title,
    a.status,
    a.priority,
    a.kind,
    a.due_date,
    a.activity_type,
    a.recurrence
FROM activities a
WHERE a.measure_id = $1
ORDER BY
    CASE a.status
        WHEN 'overdue'     THEN 0
        WHEN 'in_progress' THEN 1
        WHEN 'planned'     THEN 2
        ELSE 3
    END,
    a.due_date ASC NULLS LAST,
    a.created_at DESC;

-- name: UpdateMeasureLastVerified :exec
UPDATE measures SET last_verified_at = NOW(), updated_at = NOW() WHERE id = $1;
