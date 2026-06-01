// Package audit provides structured, durable audit event recording.
// Every call emits a matching slog INFO line (the primary durable record) and
// also writes to the audit_log Postgres table (for querying and retention).
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
	"go.opentelemetry.io/otel/trace"
)

// Event carries the event name and optional extra attributes.
// Context fields (user_id, source_ip, user_agent, request_id, trace_id) are
// extracted from ctx automatically by Record — callers need not set them.
type Event struct {
	Event string
	Attrs map[string]any // serialized to JSONB in the DB; nil is fine
}

// Record writes one audit event to the DB via q and emits a matching slog INFO
// line. q may be tx-wrapped (via q.WithTx(tx)) for atomicity with surrounding
// DB writes.
//
// The slog line is always emitted, even when the DB write fails. DB errors are
// returned but not fatal — the slog line is the authoritative audit record.
func Record(ctx context.Context, q db.Querier, e Event) error {
	// Extract context-sourced fields.
	var userID uuid.NullUUID
	if u, ok := middleware.FromContext(ctx); ok && u.ID != "" {
		if id, err := uuid.Parse(u.ID); err == nil {
			userID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}

	sourceIP := obs.SourceIPFromContext(ctx)
	userAgent := obs.UserAgentFromContext(ctx)

	var requestID string
	if rid, ok := obs.RequestIDFromContext(ctx); ok {
		requestID = rid
	}

	var traceID string
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		traceID = sc.TraceID().String()
	}

	// Marshal attrs; default to empty JSON object when nil.
	var attrsJSON json.RawMessage
	if e.Attrs != nil {
		b, err := json.Marshal(e.Attrs)
		if err == nil {
			attrsJSON = b
		} else {
			attrsJSON = json.RawMessage("{}")
		}
	} else {
		attrsJSON = json.RawMessage("{}")
	}

	// Emit the slog audit line first — it is the durable record.
	obs.Logger(ctx).InfoContext(ctx, e.Event,
		"log_type", "audit",
		"event", e.Event,
		"user_id", userID.UUID.String(),
		"source_ip", sourceIP,
		"user_agent", userAgent,
		"request_id", requestID,
		"trace_id", traceID,
		"attrs", e.Attrs,
	)

	// Write to the audit_log table.
	return q.InsertAuditLog(ctx, db.InsertAuditLogParams{ //nolint:wrapcheck
		Event:     e.Event,
		UserID:    userID,
		SourceIp:  sourceIP,
		UserAgent: userAgent,
		RequestID: requestID,
		TraceID:   traceID,
		Attrs:     attrsJSON,
	})
}

// RecordOrWarn records an audit event and logs a warning on DB write failure.
// Use this in handlers where audit persistence failures should not fail the
// user-facing operation, while still keeping observability of dropped records.
func RecordOrWarn(ctx context.Context, q db.Querier, e Event) {
	if err := Record(ctx, q, e); err != nil {
		obs.Logger(ctx).WarnContext(ctx, "audit write failed",
			"event", e.Event,
			"error", err,
		)
	}
}
