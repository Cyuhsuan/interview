// Package calendar provides the outbox delivery Adapter implementation(s)
// consumed by internal/service/calendar and cmd/calendar-worker.
package calendar

import (
	"context"
	"fmt"

	calendarsvc "backend/internal/service/calendar"
)

// SandboxAdapter is a deterministic, no-network stand-in for a real
// Google/Microsoft Calendar client. It exists because the OAuth mode,
// tenant permissions and credential storage a real client would need are
// still pending clinic confirmation (backend/README.md's 待診所確認 item
// 2) — this lets the outbox/worker/reconciliation machinery around it be
// built and tested now without pretending a production integration exists.
// It must never be described as production-ready Calendar sync.
type SandboxAdapter struct{}

func NewSandboxAdapter() *SandboxAdapter { return &SandboxAdapter{} }

var _ calendarsvc.Adapter = (*SandboxAdapter)(nil)

// Create always "succeeds" and returns a deterministic reference derived
// from the request's idempotency key, so repeated delivery of the same
// outbox row is trivially idempotent even against this fake.
func (a *SandboxAdapter) Create(_ context.Context, req calendarsvc.CreateEventRequest) (string, error) {
	return fmt.Sprintf("sandbox:%s:%s", req.Provider, req.IdempotencyKey), nil
}

func (a *SandboxAdapter) Health(_ context.Context) error { return nil }
