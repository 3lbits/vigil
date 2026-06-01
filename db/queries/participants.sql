-- name: IsParticipant :one
SELECT EXISTS(
    SELECT 1 FROM resource_participants
    WHERE resource_type = $1 AND resource_id = $2 AND user_id = $3
) AS is_participant;

-- name: AddParticipant :exec
INSERT INTO resource_participants (resource_type, resource_id, user_id, role)
VALUES ($1, $2, $3, $4)
ON CONFLICT (resource_type, resource_id, user_id) DO NOTHING;
