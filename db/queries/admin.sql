-- name: ListAllRequirements :many
SELECT
    r.id,
    r.framework_id,
    r.ref,
    r.title,
    r.description,
    r.sort_order,
    r.created_at,
    r.updated_at,
    f.name       AS framework_name,
    f.short_name AS framework_short_name
FROM requirements r
JOIN frameworks f ON f.id = r.framework_id
ORDER BY f.name, r.sort_order, r.ref;
