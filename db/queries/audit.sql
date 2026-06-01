-- name: InsertAuditLog :exec
INSERT INTO audit_log (event, user_id, source_ip, user_agent, request_id, trace_id, attrs)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditLogForMeasure :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('measure_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForActivity :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('activity_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForRequirement :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('requirement_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForFramework :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('framework_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForAsset :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('asset_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForRisk :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('risk_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogForAssessment :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.attrs @> jsonb_build_object('assessment_id', $1::text)
ORDER BY al.event_time DESC
LIMIT 50;

-- name: ListAuditLogAdmin :many
SELECT al.event_time, al.event, COALESCE(u.name, '') AS user_name, al.attrs
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
ORDER BY al.event_time DESC
LIMIT 500;
