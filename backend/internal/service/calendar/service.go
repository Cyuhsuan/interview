// Package calendar implements the outbox delivery worker logic described in
// backend/README.md's "PostgreSQL-first 預約一致性" steps 5-6 and "Calendar
// Adapter Contract": lock a due appointment_outbox row, hand it to a
// provider-neutral Adapter, and record the result. It does not create the
// initial pending rows — internal/service/booking does that inside the same
// transaction that confirms the appointment — and it never changes
// appointment status (see backend/README.md's module responsibility table).
package calendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/model"
)

// RetryableError marks an Adapter failure the worker should retry (timeout,
// throttling, transient provider error) as opposed to a permanent failure
// (invalid request, revoked credential) that should go straight to
// dead_letter. Adapters must wrap retryable causes with this type.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// CreateEventRequest is the provider-neutral input to Adapter.Create, built
// entirely from PostgreSQL appointment data — no external Calendar read
// ever informs it, per README's PostgreSQL-first rule.
type CreateEventRequest struct {
	AppointmentID  string
	Provider       string
	IdempotencyKey string
	ProfessionalID string
	PatientName    string
	PatientEmail   string
	Start          time.Time
	End            time.Time
	TimeZone       string
}

// Adapter is the Calendar Adapter Contract port: "第一版支援 Create、health、
// retry classification 與 reconciliation；不提供 Busy、Update 或 Cancel。"
type Adapter interface {
	Create(ctx context.Context, req CreateEventRequest) (eventReference string, err error)
	Health(ctx context.Context) error
}

// OutboxRepository is the port internal/repository/calendar implements.
type OutboxRepository interface {
	// LockAndMarkProcessing locks the next due (pending/retryable,
	// next_attempt_at <= now) row FOR UPDATE SKIP LOCKED, marks it
	// processing, increments attempt_count and commits — a separate,
	// short transaction from the external Calendar call that follows, per
	// README step 5. Returns nil, nil when no row is due.
	LockAndMarkProcessing(ctx context.Context) (*model.AppointmentOutbox, error)
	MarkDelivered(ctx context.Context, id, eventReference string) error
	MarkRetryable(ctx context.Context, id string, nextAttemptAt time.Time, lastError string) error
	MarkDeadLetter(ctx context.Context, id string, lastError string) error
	GetAppointment(ctx context.Context, appointmentID string) (*model.Appointment, error)
	ListForAppointment(ctx context.Context, appointmentID string) ([]model.AppointmentOutbox, error)
	ListDeadLetter(ctx context.Context) ([]model.AppointmentOutbox, error)
}

type Service struct {
	repo         OutboxRepository
	adapters     map[string]Adapter
	maxAttempts  int
	retryBackoff time.Duration
	now          func() time.Time
}

func NewService(repo OutboxRepository, adapters map[string]Adapter, maxAttempts int, retryBackoff time.Duration) *Service {
	return &Service{
		repo:         repo,
		adapters:     adapters,
		maxAttempts:  maxAttempts,
		retryBackoff: retryBackoff,
		now:          time.Now,
	}
}

// ProcessOne attempts delivery of at most one due outbox row. processed is
// false only when the queue currently has no due work.
func (s *Service) ProcessOne(ctx context.Context) (processed bool, err error) {
	rec, err := s.repo.LockAndMarkProcessing(ctx)
	if err != nil {
		return false, fmt.Errorf("lock next outbox row: %w", err)
	}
	if rec == nil {
		return false, nil
	}

	appt, err := s.repo.GetAppointment(ctx, rec.AppointmentID)
	if err != nil {
		return true, fmt.Errorf("get appointment for outbox row: %w", err)
	}
	if appt == nil {
		return true, s.repo.MarkDeadLetter(ctx, rec.ID, "appointment not found")
	}

	adapter, ok := s.adapters[rec.Provider]
	if !ok {
		return true, s.repo.MarkDeadLetter(ctx, rec.ID, "no adapter configured for provider")
	}

	eventRef, deliverErr := adapter.Create(ctx, CreateEventRequest{
		AppointmentID:  appt.ID,
		Provider:       rec.Provider,
		IdempotencyKey: rec.IdempotencyKey,
		ProfessionalID: appt.ProfessionalID,
		PatientName:    appt.PatientName,
		PatientEmail:   appt.PatientEmail,
		Start:          appt.StartAt,
		End:            appt.EndAt,
		TimeZone:       appt.TimeZone,
	})
	if deliverErr == nil {
		return true, s.repo.MarkDelivered(ctx, rec.ID, eventRef)
	}

	var retryable *RetryableError
	if errors.As(deliverErr, &retryable) && rec.AttemptCount < s.maxAttempts {
		next := s.now().Add(backoff(rec.AttemptCount, s.retryBackoff))
		return true, s.repo.MarkRetryable(ctx, rec.ID, next, classify(deliverErr))
	}
	return true, s.repo.MarkDeadLetter(ctx, rec.ID, classify(deliverErr))
}

// backoff grows exponentially with attempt count: base, 2x, 4x, ... This is
// an operational parameter, not a business rule, and is intentionally kept
// simple (no jitter) for this stage.
func backoff(attemptCount int, base time.Duration) time.Duration {
	d := base
	for range attemptCount {
		d *= 2
	}
	return d
}

// classify avoids storing raw provider response bodies in last_error, per
// README's security baseline ("不得包含...provider response body").
// Adapters are expected to return errors with an actionable message already
// free of patient data or credentials; this only guards against unbounded
// length.
func classify(err error) string {
	msg := err.Error()
	const maxLen = 500
	if len(msg) > maxLen {
		return msg[:maxLen]
	}
	return msg
}

// DeliveryStatus computes the API-facing calendarDelivery value for an
// appointment from its two outbox rows, per README's "防止重疊與狀態"
// mapping: both delivered -> delivered; exactly one delivered -> partial;
// any dead_letter -> attentionRequired; otherwise -> queued.
func (s *Service) DeliveryStatus(ctx context.Context, appointmentID string) (string, error) {
	rows, err := s.repo.ListForAppointment(ctx, appointmentID)
	if err != nil {
		return "", fmt.Errorf("list outbox rows: %w", err)
	}
	delivered := 0
	for _, row := range rows {
		if row.Status == model.OutboxStatusDeadLetter {
			return "attentionRequired", nil
		}
		if row.Status == model.OutboxStatusDelivered {
			delivered++
		}
	}
	switch delivered {
	case len(rows):
		if len(rows) == 0 {
			return "queued", nil
		}
		return "delivered", nil
	case 0:
		return "queued", nil
	default:
		return "partial", nil
	}
}

// Reconcile returns the current dead_letter backlog for the caller to
// surface as an operational alert. Actual alert routing/SLA is still
// pending clinic confirmation (README's 待診所確認 item 3), so this
// deliberately only reports the backlog rather than paging anyone itself.
func (s *Service) Reconcile(ctx context.Context) ([]model.AppointmentOutbox, error) {
	rows, err := s.repo.ListDeadLetter(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dead letter rows: %w", err)
	}
	return rows, nil
}

// OutboxIdempotencyKey derives the stable per-appointment-per-provider key
// used both as the outbox row's dedup key and as the idempotency key sent
// to the provider adapter, per README step 5 ("以 appointment ID 與
// provider 派生的穩定 idempotency key").
func OutboxIdempotencyKey(appointmentID, provider string) string {
	return fmt.Sprintf("appt:%s:%s", appointmentID, provider)
}
