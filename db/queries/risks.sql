-- name: GetRiskGlobalSettings :one
SELECT * FROM risk_global_settings WHERE id = 1;

-- name: ListRiskScaleLabels :many
SELECT * FROM risk_scale_labels ORDER BY scale DESC, level ASC;

-- name: UpsertRiskScaleLabel :exec
INSERT INTO risk_scale_labels (scale, level, label, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (scale, level) DO UPDATE
    SET label = EXCLUDED.label, description = EXCLUDED.description;

-- name: UpdateRiskGlobalSettings :exec
UPDATE risk_global_settings
SET acceptance_criteria = $1, low_max = $2, high_min = $3
WHERE id = 1;

-- name: ListParticipantsForAssessment :many
SELECT u.*
FROM users u
JOIN risk_assessment_participants rap ON rap.user_id = u.id
WHERE rap.assessment_id = $1
ORDER BY u.name;

-- name: ClearAssessmentParticipants :exec
DELETE FROM risk_assessment_participants WHERE assessment_id = $1;

-- name: AddAssessmentParticipant :exec
INSERT INTO risk_assessment_participants (assessment_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveAssessmentParticipant :exec
DELETE FROM risk_assessment_participants
WHERE assessment_id = $1 AND user_id = $2;

-- name: ListRiskAssessments :many
SELECT * FROM risk_assessments ORDER BY updated_at DESC;

-- name: ListRiskAssessmentsForUser :many
SELECT DISTINCT ra.*
FROM risk_assessments ra
LEFT JOIN risk_assessment_participants rap ON rap.assessment_id = ra.id
WHERE ra.is_public = true
   OR ra.risk_owner_id = $1
   OR ra.created_by = $1
   OR rap.user_id = $1
ORDER BY ra.updated_at DESC;

-- name: GetRiskAssessment :one
SELECT * FROM risk_assessments WHERE id = $1;

-- name: IsRiskAssessmentAccessible :one
SELECT EXISTS(
    SELECT 1 FROM risk_assessments ra
    LEFT JOIN risk_assessment_participants rap ON rap.assessment_id = ra.id
    WHERE ra.id = $1
    AND (ra.is_public = true OR ra.risk_owner_id = $2::uuid OR ra.created_by = $2::uuid OR rap.user_id = $2::uuid)
) AS accessible;

-- name: IsRiskAssessmentParticipant :one
SELECT EXISTS(
    SELECT 1
    FROM risk_assessment_participants
    WHERE assessment_id = $1 AND user_id = $2
) AS is_participant;

-- name: ToggleRiskAssessmentPublic :one
UPDATE risk_assessments
SET is_public = NOT is_public, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListAllRisksForUser :many
SELECT r.*, ra.name AS assessment_name, ra.ref_num AS assessment_ref_num
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
WHERE ra.is_public = true
   OR ra.risk_owner_id = $1::uuid
   OR ra.created_by = $1::uuid
   OR EXISTS (
       SELECT 1 FROM risk_assessment_participants rap
       WHERE rap.assessment_id = ra.id AND rap.user_id = $1::uuid
   )
ORDER BY COALESCE(r.likelihood_current, 0) * COALESCE(r.consequence_current, 0) DESC, r.created_at DESC;

-- name: CreateRiskAssessment :one
INSERT INTO risk_assessments (
    name, scope, analysis_object, security_objectives, business_objectives,
    type, risk_owner_id, org_id, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateRiskAssessmentStep1 :one
UPDATE risk_assessments
SET name = $2, scope = $3, analysis_object = $4, security_objectives = $5,
    business_objectives = $6, type = $7, risk_owner_id = $8, org_id = $9,
    current_step = GREATEST(current_step, 2), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateRiskAssessmentStep :exec
UPDATE risk_assessments
SET current_step = $2, status = $3, updated_at = NOW()
WHERE id = $1;

-- name: UpdateRiskAssessmentReviewed :exec
UPDATE risk_assessments
SET last_reviewed_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: DeleteRiskAssessment :exec
DELETE FROM risk_assessments WHERE id = $1;

-- name: ListRisksForAssessment :many
SELECT * FROM risks
WHERE assessment_id = $1
ORDER BY COALESCE(likelihood_current, 0) * COALESCE(consequence_current, 0) DESC, created_at DESC;

-- name: ListAllRisks :many
SELECT r.*, ra.name AS assessment_name, ra.ref_num AS assessment_ref_num
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
ORDER BY COALESCE(r.likelihood_current, 0) * COALESCE(r.consequence_current, 0) DESC, r.created_at DESC;

-- name: ListOwnedRisks :many
SELECT
    r.id,
    r.assessment_id,
    r.name,
    COALESCE(r.likelihood_current, 0) AS likelihood_current,
    COALESCE(r.consequence_current, 0) AS consequence_current,
    to_char(r.updated_at, 'YYYY-MM-DD HH24:MI') AS updated_at,
    ra.name AS assessment_name,
    COALESCE(ra.ref_num, 0) AS assessment_ref_num
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
WHERE r.owner_id = $1
ORDER BY r.updated_at DESC
LIMIT 25;

-- name: GetRisk :one
SELECT * FROM risks WHERE id = $1;

-- name: CreateRisk :one
INSERT INTO risks (assessment_id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateRiskIdentification :one
UPDATE risks
SET name = $2, description = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateRiskCurrentScores :one
UPDATE risks
SET likelihood_current = $2, consequence_current = $3,
    likelihood_reasoning = $4, consequence_reasoning = $5,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ReassessRiskCurrentScores :one
UPDATE risks
SET likelihood_current = $2,
    consequence_current = $3,
    assessment_rationale = $4,
    assessed_at = NOW(),
    assessed_by = $5,
    review_needed = FALSE,
    review_due = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateRiskTargetScore :exec
UPDATE risks
SET likelihood_target = $2, consequence_target = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateRiskDecision :exec
UPDATE risks
SET risk_decision = $2, decision_notes = $3, updated_at = NOW()
WHERE id = $1;

-- name: DeleteRisk :exec
DELETE FROM risks WHERE id = $1;

-- name: CountRisksForMatrix :many
SELECT likelihood_current, consequence_current, COUNT(*) AS count
FROM risks
WHERE likelihood_current IS NOT NULL AND consequence_current IS NOT NULL
GROUP BY likelihood_current, consequence_current;

-- name: ListTopRisks :many
SELECT r.*, ra.name AS assessment_name
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
WHERE r.likelihood_current IS NOT NULL AND r.consequence_current IS NOT NULL
ORDER BY r.likelihood_current * r.consequence_current DESC
LIMIT 5;

-- name: GetRiskStats :one
SELECT
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE likelihood_current * consequence_current >= 12) AS red_count,
    COUNT(*) FILTER (WHERE likelihood_current * consequence_current BETWEEN 6 AND 11) AS yellow_count,
    COUNT(*) FILTER (WHERE likelihood_current * consequence_current BETWEEN 1 AND 5) AS green_count
FROM risks
WHERE likelihood_current IS NOT NULL AND consequence_current IS NOT NULL;

-- name: LinkRiskToMeasure :exec
INSERT INTO risk_measure_links (risk_id, measure_id, note)
VALUES ($1, $2, $3)
ON CONFLICT (risk_id, measure_id) DO NOTHING;

-- name: UnlinkRiskFromMeasure :exec
DELETE FROM risk_measure_links WHERE risk_id = $1 AND measure_id = $2;

-- name: ListMeasuresForRisk :many
SELECT m.* FROM measures m
JOIN risk_measure_links rml ON rml.measure_id = m.id
WHERE rml.risk_id = $1
ORDER BY m.name;

-- name: LinkRiskToAsset :exec
INSERT INTO risk_asset_links (risk_id, asset_id)
VALUES ($1, $2)
ON CONFLICT (risk_id, asset_id) DO NOTHING;

-- name: UnlinkRiskFromAsset :exec
DELETE FROM risk_asset_links WHERE risk_id = $1 AND asset_id = $2;

-- name: ClearRiskAssets :exec
DELETE FROM risk_asset_links WHERE risk_id = $1;

-- name: ListAssetsForRisk :many
SELECT a.*
FROM assets a
JOIN risk_asset_links ral ON ral.asset_id = a.id
WHERE ral.risk_id = $1
ORDER BY a.name;

-- name: SearchAssetsForRisk :many
SELECT a.*
FROM assets a
WHERE
    (sqlc.arg(q) = '' OR a.name ILIKE '%' || sqlc.arg(q) || '%' OR a.owner ILIKE '%' || sqlc.arg(q) || '%')
    AND NOT EXISTS (
        SELECT 1 FROM risk_asset_links ral
        WHERE ral.risk_id = sqlc.arg(risk_id)
          AND ral.asset_id = a.id
    )
ORDER BY a.name
LIMIT sqlc.arg(limit_count);

-- name: ListRisksForMeasure :many
SELECT r.* FROM risks r
JOIN risk_measure_links rml ON rml.risk_id = r.id
WHERE rml.measure_id = $1
ORDER BY r.name;

-- name: FlagRiskForReview :exec
UPDATE risks
SET review_needed = TRUE,
    review_due = COALESCE(review_due, NOW()),
    updated_at = NOW()
WHERE id = $1;

-- name: ListRiskReviewQueue :many
SELECT r.*, ra.name AS assessment_name, ra.ref_num AS assessment_ref_num
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
WHERE r.review_needed = TRUE
ORDER BY COALESCE(r.review_due, r.updated_at) ASC, r.updated_at DESC;

-- name: ListRiskReviewQueueForUser :many
SELECT DISTINCT r.*, ra.name AS assessment_name, ra.ref_num AS assessment_ref_num
FROM risks r
JOIN risk_assessments ra ON ra.id = r.assessment_id
LEFT JOIN risk_assessment_participants rap ON rap.assessment_id = ra.id
WHERE r.review_needed = TRUE
  AND (
      ra.is_public = TRUE
      OR ra.risk_owner_id = $1::uuid
      OR ra.created_by = $1::uuid
      OR rap.user_id = $1::uuid
  )
ORDER BY COALESCE(r.review_due, r.updated_at) ASC, r.updated_at DESC;

-- name: CreateRiskReassessmentEvent :exec
INSERT INTO risk_reassessment_events (risk_id, measure_id, trigger_status, triggered_by, note)
VALUES ($1, $2, $3, $4, $5);

-- name: ListRiskReassessmentEvents :many
SELECT
    rre.id,
    rre.risk_id,
    rre.measure_id,
    rre.trigger_status,
    rre.triggered_at,
    rre.triggered_by,
    rre.note,
    m.name AS measure_name,
    m.ref_num AS measure_ref_num
FROM risk_reassessment_events rre
JOIN measures m ON m.id = rre.measure_id
WHERE rre.risk_id = sqlc.arg(risk_id)
  AND (
      sqlc.narg(since_at)::timestamptz IS NULL
      OR rre.triggered_at > sqlc.narg(since_at)::timestamptz
  )
ORDER BY rre.triggered_at DESC;

-- name: ListMeasureRiskLinkIDs :many
SELECT DISTINCT measure_id FROM risk_measure_links ORDER BY measure_id;

-- name: SearchMeasures :many
SELECT * FROM measures
WHERE name ILIKE $1
ORDER BY name
LIMIT 10;

-- name: LinkRiskToActivity :exec
INSERT INTO risk_activity_links (risk_id, activity_id)
VALUES ($1, $2)
ON CONFLICT (risk_id, activity_id) DO NOTHING;

-- name: UnlinkRiskFromActivity :exec
DELETE FROM risk_activity_links WHERE risk_id = $1 AND activity_id = $2;

-- name: ListActivitiesForRisk :many
SELECT a.* FROM activities a
JOIN risk_activity_links ral ON ral.activity_id = a.id
WHERE ral.risk_id = $1
ORDER BY a.title;

-- name: ListRisksForActivity :many
SELECT r.* FROM risks r
JOIN risk_activity_links ral ON ral.risk_id = r.id
WHERE ral.activity_id = $1
ORDER BY r.name;

-- name: SearchActivities :many
SELECT a.* FROM activities a
WHERE a.title ILIKE $1
ORDER BY a.title
LIMIT 10;

-- name: AcceptAssessment :one
UPDATE risk_assessments
SET status = 'active', acceptance_note = '', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeclineAssessment :one
UPDATE risk_assessments
SET status = 'draft', acceptance_note = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
