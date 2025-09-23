package library

import (
	"context"
	"time"
)

const (
	// AuditEventImportCompleted indicates that an import job finished successfully.
	AuditEventImportCompleted = "library.import.completed"
)

// AuditEvent captures notable actions for downstream notification or analytics systems.
type AuditEvent struct {
	EventType   string
	JobID       string
	JobType     string
	TargetID    string
	EntityType  string
	CompletedAt time.Time
}

// AuditSink publishes audit events to an external system.
type AuditSink interface {
	Publish(context.Context, AuditEvent) error
}

type noopAuditSink struct{}

func (noopAuditSink) Publish(context.Context, AuditEvent) error { return nil }

// AuditSinkFunc adapts a function into an AuditSink implementation.
type AuditSinkFunc func(context.Context, AuditEvent) error

// Publish dispatches the event to the underlying function.
func (fn AuditSinkFunc) Publish(ctx context.Context, event AuditEvent) error {
	return fn(ctx, event)
}
