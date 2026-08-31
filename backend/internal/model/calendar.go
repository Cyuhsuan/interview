package model

import "time"

const (
	CalendarProviderGoogle    = "google"
	CalendarProviderMicrosoft = "microsoft"

	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusRetryable  = "retryable"
	OutboxStatusDelivered  = "delivered"
	OutboxStatusDeadLetter = "dead_letter"
)

// AppointmentOutbox is one provider's delivery record for an Appointment,
// per README's "PostgreSQL-first 預約一致性" steps 3-6. Two rows (google,
// microsoft) are created per appointment inside the same transaction that
// confirms it; internal/service/calendar owns the state machine that
// advances status from here on.
type AppointmentOutbox struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	AppointmentID  string    `gorm:"column:appointment_id;type:uuid;not null"`
	Provider       string    `gorm:"column:provider;size:16;not null"`
	Status         string    `gorm:"column:status;size:16;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:128;not null"`
	AttemptCount   int       `gorm:"column:attempt_count;not null"`
	NextAttemptAt  time.Time `gorm:"column:next_attempt_at;not null"`
	EventReference *string   `gorm:"column:event_reference;size:512"`
	LastError      *string   `gorm:"column:last_error;size:500"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (AppointmentOutbox) TableName() string { return "appointment_outbox" }
