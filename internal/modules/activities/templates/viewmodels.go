package activitiestemplates

import (
	"database/sql"
	"time"
)

// FormatDate formats a nullable date for display.
func FormatDate(t sql.NullTime) string {
	if !t.Valid {
		return "—"
	}
	return t.Time.Format("2 Jan 2006")
}

// FormatDateTime formats a nullable timestamp for display.
func FormatDateTime(t sql.NullTime) string {
	if !t.Valid {
		return "—"
	}
	return t.Time.Format("2 Jan 2006, 15:04")
}

// DateInputVal formats a nullable date as YYYY-MM-DD for <input type="date">.
func DateInputVal(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02")
}

// IsOverdue returns true if the activity should be shown as overdue.
func IsOverdue(status string, dueDate sql.NullTime) bool {
	if status == "completed" || status == "overdue" {
		return status == "overdue"
	}
	if !dueDate.Valid {
		return false
	}
	return dueDate.Time.Before(time.Now().Truncate(24 * time.Hour))
}
