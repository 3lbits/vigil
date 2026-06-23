-- name: ListAvvik :many
SELECT *
FROM avvik
WHERE
    (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text) AND
    (sqlc.narg(risk_level)::text IS NULL OR risk_level = sqlc.narg(risk_level)::text) AND
    (sqlc.narg(personal_data)::bool IS NULL OR personal_data = sqlc.narg(personal_data)::bool) AND
    (sqlc.narg(ksi)::bool IS NULL OR ksi = sqlc.narg(ksi)::bool) AND
    (sqlc.narg(market_sensitive)::bool IS NULL OR market_sensitive = sqlc.narg(market_sensitive)::bool) AND
    (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id)::uuid) AND
    (NOT sqlc.arg(mine)::bool OR assigned_to = sqlc.arg(assignee_id)::uuid) AND
    (sqlc.arg(q) = '' OR title ILIKE '%' || sqlc.arg(q) || '%' OR description ILIKE '%' || sqlc.arg(q) || '%')
ORDER BY updated_at DESC
LIMIT sqlc.arg(page_size)
OFFSET sqlc.arg(page_offset);

-- name: GetAvvik :one
SELECT * FROM avvik WHERE id = $1;

-- name: CreateAvvik :one
INSERT INTO avvik (
    title, description, discovered_at, reported_at, reporter_name, reporter_email,
    assigned_to, org_unit_id, risk_level, status, personal_data, ksi, ksi_information_owner,
    market_sensitive, market_assessment_note, gdpr_deadline_at, realised_risk_id,
    import_source, external_reference, imported_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20
)
RETURNING *;

-- name: UpdateAvvikTriage :one
UPDATE avvik
SET risk_level = $2,
    personal_data = $3,
    ksi = $4,
    ksi_information_owner = $5,
    market_sensitive = $6,
    market_assessment_note = $7,
    gdpr_deadline_at = $8,
    org_unit_id = $9,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateAvvikStatus :one
UPDATE avvik
SET status = $2,
    closed_at = $3,
    closure_summary = $4,
    root_cause = $5,
    lessons_learned = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateAvvikClosureFlags :one
UPDATE avvik
SET log_qa_done = $2,
    followups_delegated = $3,
    reporter_informed = $4,
    org_informed = $5,
    mgmt_informed = $6,
    decisions_anchored = $7,
    implementation_deadline_set = $8,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: LinkAvvikMeasure :exec
INSERT INTO avvik_measures (avvik_id, measure_id, relationship)
VALUES ($1, $2, $3)
ON CONFLICT (avvik_id, measure_id) DO UPDATE SET relationship = EXCLUDED.relationship;

-- name: UnlinkAvvikMeasure :exec
DELETE FROM avvik_measures WHERE avvik_id = $1 AND measure_id = $2;

-- name: ListAvvikMeasures :many
SELECT m.*
FROM avvik_measures am
JOIN measures m ON m.id = am.measure_id
WHERE am.avvik_id = $1
ORDER BY m.updated_at DESC;

-- name: LinkAvvikActivity :exec
INSERT INTO avvik_activities (avvik_id, activity_id)
VALUES ($1, $2)
ON CONFLICT (avvik_id, activity_id) DO NOTHING;

-- name: UnlinkAvvikActivity :exec
DELETE FROM avvik_activities WHERE avvik_id = $1 AND activity_id = $2;

-- name: ListAvvikActivities :many
SELECT a.*
FROM avvik_activities aa
JOIN activities a ON a.id = aa.activity_id
WHERE aa.avvik_id = $1
ORDER BY a.updated_at DESC;

-- name: AddAvvikNotification :one
INSERT INTO avvik_notifications (avvik_id, audience, sent_at, sent_by, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAvvikNotifications :many
SELECT * FROM avvik_notifications WHERE avvik_id = $1 ORDER BY sent_at DESC;

-- name: AddAvvikAttachment :one
INSERT INTO avvik_attachments (avvik_id, filename, storage_key, uploaded_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAvvikAttachments :many
SELECT * FROM avvik_attachments WHERE avvik_id = $1 ORDER BY uploaded_at DESC;

-- name: AddAvvikEvent :one
INSERT INTO avvik_events (avvik_id, actor_id, actor_label, event_type, payload, occurred_at, import_source)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListAvvikEvents :many
SELECT * FROM avvik_events WHERE avvik_id = $1 ORDER BY occurred_at DESC;
