-- name: ListAssets :many
SELECT *
FROM assets
WHERE
    (sqlc.arg(q) = '' OR name ILIKE '%' || sqlc.arg(q) || '%' OR owner ILIKE '%' || sqlc.arg(q) || '%')
    AND (sqlc.arg(status) = '' OR status = sqlc.arg(status))
ORDER BY name
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: GetAsset :one
SELECT * FROM assets WHERE id = $1;

-- name: CreateAsset :one
INSERT INTO assets (name, description, asset_type, owner, status, criticality)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateAsset :one
UPDATE assets
SET
    name = $2,
    description = $3,
    asset_type = $4,
    owner = $5,
    status = $6,
    criticality = $7,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAsset :exec
DELETE FROM assets WHERE id = $1;

-- name: ListAssetsForAssessment :many
SELECT a.*
FROM assets a
JOIN risk_assessment_assets raa ON raa.asset_id = a.id
WHERE raa.assessment_id = $1
ORDER BY a.name;

-- name: AddAssetToAssessment :exec
INSERT INTO risk_assessment_assets (assessment_id, asset_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveAssetFromAssessment :exec
DELETE FROM risk_assessment_assets
WHERE assessment_id = $1 AND asset_id = $2;

-- name: SearchAssetsToLink :many
SELECT a.*
FROM assets a
WHERE
    (sqlc.arg(q) = '' OR a.name ILIKE '%' || sqlc.arg(q) || '%' OR a.owner ILIKE '%' || sqlc.arg(q) || '%')
    AND NOT EXISTS (
        SELECT 1 FROM risk_assessment_assets raa
        WHERE raa.assessment_id = sqlc.arg(assessment_id)
          AND raa.asset_id = a.id
    )
ORDER BY a.name
LIMIT sqlc.arg(limit_count);
