-- name: UpsertUser :one
INSERT INTO users (provider, provider_id, email, name, role)
VALUES ($1, $2, $3, $4,
    CASE WHEN (SELECT COUNT(*) FROM users) = 0 THEN 'admin' ELSE 'viewer' END
)
ON CONFLICT (provider, provider_id) DO UPDATE
    SET email      = EXCLUDED.email,
        name       = EXCLUDED.name,
        updated_at = NOW()
RETURNING *;

-- name: SetUserRole :one
UPDATE users
SET role = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at ASC;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: PreCreateUser :one
INSERT INTO users (provider, provider_id, email, name, role)
VALUES ('pending', $1, $1, $2, $3)
ON CONFLICT (provider, provider_id) DO UPDATE
    SET role       = EXCLUDED.role,
        name       = EXCLUDED.name,
        updated_at = NOW()
RETURNING *;

-- name: ClaimPendingUser :one
UPDATE users
SET provider    = $2,
    provider_id = $3,
    name        = $4,
    updated_at  = NOW()
WHERE email = $1
  AND provider = 'pending'
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: ListActiveSessionsByUser :many
SELECT
    u.id         AS user_id,
    u.name,
    u.email,
    u.role,
    COUNT(s.token)::bigint     AS session_count,
    MAX(s.expiry)::timestamptz AS expires_at
FROM users u
JOIN sessions s ON s.user_id = u.id
WHERE s.expiry > NOW()
GROUP BY u.id, u.name, u.email, u.role
ORDER BY expires_at DESC;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: SetUserOrg :one
UPDATE users SET org_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpsertDevStubUser :one
INSERT INTO users (provider, provider_id, email, name, role)
VALUES ('dev_stub', $1, $2, $3, $4)
ON CONFLICT (provider, provider_id) DO UPDATE
    SET email      = EXCLUDED.email,
        name       = EXCLUDED.name,
        role       = EXCLUDED.role,
        updated_at = NOW()
RETURNING *;

-- name: ListDevStubUsers :many
SELECT * FROM users
WHERE provider = 'dev_stub'
ORDER BY
    CASE role
        WHEN 'admin' THEN 0
        WHEN 'editor' THEN 1
        WHEN 'contributor' THEN 2
        WHEN 'viewer' THEN 3
        ELSE 4
    END,
    created_at ASC;
