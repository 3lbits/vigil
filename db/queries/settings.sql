-- name: GetAppSettings :one
SELECT * FROM app_settings WHERE id = 1;

-- name: UpdateAppSettings :exec
UPDATE app_settings
SET compliance_enabled = $1,
    risk_enabled       = $2,
    activities_enabled = $3,
    assets_enabled     = $4,
    playground_enabled = $5,
    avvik_enabled      = $6
WHERE id = 1;
