-- +goose Up

ALTER TABLE deadlines RENAME TO tasks;
ALTER TABLE tasks RENAME COLUMN deadline_type TO task_type;
ALTER INDEX deadlines_type_idx     RENAME TO tasks_type_idx;
ALTER INDEX deadlines_status_idx   RENAME TO tasks_status_idx;
ALTER INDEX deadlines_due_date_idx RENAME TO tasks_due_date_idx;

-- +goose Down

ALTER INDEX tasks_due_date_idx RENAME TO deadlines_due_date_idx;
ALTER INDEX tasks_status_idx   RENAME TO deadlines_status_idx;
ALTER INDEX tasks_type_idx     RENAME TO deadlines_type_idx;
ALTER TABLE tasks RENAME COLUMN task_type TO deadline_type;
ALTER TABLE tasks RENAME TO deadlines;
