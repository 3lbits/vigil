-- name: GetDashboardStats :one
SELECT
    (SELECT COUNT(*)::bigint FROM frameworks WHERE NOT not_relevant) AS frameworks_count,
    (SELECT COUNT(*)::bigint FROM requirements r
     INNER JOIN frameworks f ON r.framework_id = f.id
     WHERE NOT r.not_relevant AND NOT f.not_relevant) AS requirements_count,
    (SELECT COUNT(*)::bigint FROM measures)       AS measures_count,
    (SELECT COUNT(*)::bigint FROM measures WHERE status = 'implemented') AS implemented_count,
    (SELECT COUNT(DISTINCT r.id)::bigint
     FROM requirements r
     INNER JOIN frameworks f ON r.framework_id = f.id
     INNER JOIN measure_requirements mr ON r.id = mr.requirement_id
     INNER JOIN measures m ON mr.measure_id = m.id
     WHERE NOT r.not_relevant
       AND NOT f.not_relevant
       AND m.status = 'implemented') AS covered_requirements_count,
    (SELECT COUNT(*)::bigint FROM activities
     WHERE status = 'overdue'
        OR (due_date < CURRENT_DATE AND status NOT IN ('completed', 'overdue'))) AS overdue_activities_count,
    (SELECT COUNT(*)::bigint FROM activities
     WHERE due_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '7 days'
       AND status NOT IN ('completed', 'overdue')) AS activities_due_this_week_count;
