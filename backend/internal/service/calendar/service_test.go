package calendar

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/model"
)

type fakeRepo struct {
	rows          map[string]model.AppointmentOutbox
	order         []string
	appointments  map[string]model.Appointment
	markDelivered []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rows:         map[string]model.AppointmentOutbox{},
		appointments: map[string]model.Appointment{},
	}
}

func (r *fakeRepo) LockAndMarkProcessing(ctx context.Context) (*model.AppointmentOutbox, error) {
	now := time.Now()
	for _, id := range r.order {
		row := r.rows[id]
		if (row.Status == model.OutboxStatusPending || row.Status == model.OutboxStatusRetryable) && !row.NextAttemptAt.After(now) {
			row.Status = model.OutboxStatusProcessing
			row.AttemptCount++
			r.rows[id] = row
			cp := row
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeRepo) MarkDelivered(ctx context.Context, id, eventReference string) error {
	row := r.rows[id]
	row.Status = model.OutboxStatusDelivered
	row.EventReference = &eventReference
	r.rows[id] = row
	r.markDelivered = append(r.markDelivered, id)
	return nil
}

func (r *fakeRepo) MarkRetryable(ctx context.Context, id string, nextAttemptAt time.Time, lastError string) error {
	row := r.rows[id]
	row.Status = model.OutboxStatusRetryable
	row.NextAttemptAt = nextAttemptAt
	row.LastError = &lastError
	r.rows[id] = row
	return nil
}

func (r *fakeRepo) MarkDeadLetter(ctx context.Context, id string, lastError string) error {
	row := r.rows[id]
	row.Status = model.OutboxStatusDeadLetter
	row.LastError = &lastError
	r.rows[id] = row
	return nil
}

func (r *fakeRepo) GetAppointment(ctx context.Context, appointmentID string) (*model.Appointment, error) {
	appt, ok := r.appointments[appointmentID]
	if !ok {
		return nil, nil
	}
	return &appt, nil
}

func (r *fakeRepo) ListForAppointment(ctx context.Context, appointmentID string) ([]model.AppointmentOutbox, error) {
	var rows []model.AppointmentOutbox
	for _, id := range r.order {
		row := r.rows[id]
		if row.AppointmentID == appointmentID {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (r *fakeRepo) ListDeadLetter(ctx context.Context) ([]model.AppointmentOutbox, error) {
	var rows []model.AppointmentOutbox
	for _, id := range r.order {
		if r.rows[id].Status == model.OutboxStatusDeadLetter {
			rows = append(rows, r.rows[id])
		}
	}
	return rows, nil
}

func (r *fakeRepo) addRow(row model.AppointmentOutbox) {
	r.rows[row.ID] = row
	r.order = append(r.order, row.ID)
}

type fakeAdapter struct {
	err      error
	eventRef string
	calls    int
}

func (a *fakeAdapter) Create(ctx context.Context, req CreateEventRequest) (string, error) {
	a.calls++
	if a.err != nil {
		return "", a.err
	}
	return a.eventRef, nil
}

func (a *fakeAdapter) Health(ctx context.Context) error { return nil }

func newFixture(t *testing.T) (*fakeRepo, string) {
	t.Helper()
	repo := newFakeRepo()
	repo.appointments["appt-1"] = model.Appointment{ID: "appt-1", ProfessionalID: "prof-1", PatientName: "Jane", PatientEmail: "jane@example.com"}
	repo.addRow(model.AppointmentOutbox{
		ID: "row-1", AppointmentID: "appt-1", Provider: model.CalendarProviderGoogle,
		Status: model.OutboxStatusPending, IdempotencyKey: OutboxIdempotencyKey("appt-1", model.CalendarProviderGoogle),
		NextAttemptAt: time.Now().Add(-time.Second),
	})
	return repo, "row-1"
}

func TestProcessOne_Success(t *testing.T) {
	repo, rowID := newFixture(t)
	adapter := &fakeAdapter{eventRef: "event-ref"}
	svc := NewService(repo, map[string]Adapter{model.CalendarProviderGoogle: adapter}, 3, time.Second)

	processed, err := svc.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessOne() = (%v, %v), want (true, nil)", processed, err)
	}
	if got := repo.rows[rowID].Status; got != model.OutboxStatusDelivered {
		t.Fatalf("status = %q, want delivered", got)
	}
	if got := repo.rows[rowID].EventReference; got == nil || *got != "event-ref" {
		t.Fatalf("event reference = %v, want event-ref", got)
	}
}

func TestProcessOne_EmptyQueue(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, map[string]Adapter{}, 3, time.Second)

	processed, err := svc.ProcessOne(context.Background())
	if err != nil || processed {
		t.Fatalf("ProcessOne() = (%v, %v), want (false, nil)", processed, err)
	}
}

func TestProcessOne_RetryableUnderLimitStaysRetryable(t *testing.T) {
	repo, rowID := newFixture(t)
	adapter := &fakeAdapter{err: &RetryableError{Err: errors.New("timeout")}}
	svc := NewService(repo, map[string]Adapter{model.CalendarProviderGoogle: adapter}, 3, time.Second)

	if _, err := svc.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	row := repo.rows[rowID]
	if row.Status != model.OutboxStatusRetryable {
		t.Fatalf("status = %q, want retryable", row.Status)
	}
	if !row.NextAttemptAt.After(time.Now()) {
		t.Fatalf("next_attempt_at = %v, want in the future", row.NextAttemptAt)
	}
	if row.LastError == nil || *row.LastError != "timeout" {
		t.Fatalf("last_error = %v, want timeout", row.LastError)
	}
}

func TestProcessOne_RetryableExhaustedGoesDeadLetter(t *testing.T) {
	repo, rowID := newFixture(t)
	row := repo.rows[rowID]
	row.AttemptCount = 3
	repo.rows[rowID] = row
	adapter := &fakeAdapter{err: &RetryableError{Err: errors.New("timeout")}}
	svc := NewService(repo, map[string]Adapter{model.CalendarProviderGoogle: adapter}, 3, time.Second)

	if _, err := svc.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if got := repo.rows[rowID].Status; got != model.OutboxStatusDeadLetter {
		t.Fatalf("status = %q, want dead_letter", got)
	}
}

func TestProcessOne_PermanentErrorGoesDeadLetterImmediately(t *testing.T) {
	repo, rowID := newFixture(t)
	adapter := &fakeAdapter{err: errors.New("invalid credential")}
	svc := NewService(repo, map[string]Adapter{model.CalendarProviderGoogle: adapter}, 3, time.Second)

	if _, err := svc.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if got := repo.rows[rowID].Status; got != model.OutboxStatusDeadLetter {
		t.Fatalf("status = %q, want dead_letter", got)
	}
}

func TestProcessOne_NoAdapterConfiguredGoesDeadLetter(t *testing.T) {
	repo, rowID := newFixture(t)
	svc := NewService(repo, map[string]Adapter{}, 3, time.Second)

	if _, err := svc.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if got := repo.rows[rowID].Status; got != model.OutboxStatusDeadLetter {
		t.Fatalf("status = %q, want dead_letter", got)
	}
}

func TestDeliveryStatus(t *testing.T) {
	tests := []struct {
		name   string
		rows   []model.AppointmentOutbox
		expect string
	}{
		{"queued when both pending", []model.AppointmentOutbox{{Status: model.OutboxStatusPending}, {Status: model.OutboxStatusPending}}, "queued"},
		{"partial when one delivered", []model.AppointmentOutbox{{Status: model.OutboxStatusDelivered}, {Status: model.OutboxStatusPending}}, "partial"},
		{"delivered when both delivered", []model.AppointmentOutbox{{Status: model.OutboxStatusDelivered}, {Status: model.OutboxStatusDelivered}}, "delivered"},
		{"attentionRequired when any dead_letter", []model.AppointmentOutbox{{Status: model.OutboxStatusDelivered}, {Status: model.OutboxStatusDeadLetter}}, "attentionRequired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			for i, row := range tt.rows {
				row.ID = "row"
				row.AppointmentID = "appt"
				_ = i
				repo.rows[row.ID+string(rune('a'+i))] = row
				repo.order = append(repo.order, row.ID+string(rune('a'+i)))
			}
			svc := NewService(repo, map[string]Adapter{}, 3, time.Second)
			got, err := svc.DeliveryStatus(context.Background(), "appt")
			if err != nil {
				t.Fatalf("DeliveryStatus() error = %v", err)
			}
			if got != tt.expect {
				t.Fatalf("DeliveryStatus() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestOutboxIdempotencyKeyIsStablePerAppointmentAndProvider(t *testing.T) {
	a := OutboxIdempotencyKey("appt-1", model.CalendarProviderGoogle)
	b := OutboxIdempotencyKey("appt-1", model.CalendarProviderGoogle)
	c := OutboxIdempotencyKey("appt-1", model.CalendarProviderMicrosoft)
	if a != b {
		t.Fatalf("key not stable: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("key not provider-specific: %q == %q", a, c)
	}
}
