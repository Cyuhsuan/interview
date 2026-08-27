// Package booking is the only place in the Booking module allowed to
// import GORM or hold a *gorm.DB reference, per backend/README.md's
// tech-stack boundaries.
package booking

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/model"
	bookingsvc "backend/internal/service/booking"
)

// postgresExclusionViolation is the SQLSTATE for a PostgreSQL exclusion
// constraint conflict (used by the appointments overlap-prevention
// constraint defined in migration 000005).
const postgresExclusionViolation = "23P01"

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

var _ bookingsvc.Repository = (*Repository)(nil)

func (r *Repository) CreateSession(ctx context.Context, session model.BookingSession) error {
	if err := r.db.WithContext(ctx).Create(&session).Error; err != nil {
		return fmt.Errorf("create booking session: %w", err)
	}
	return nil
}

func (r *Repository) GetSession(ctx context.Context, id string) (*model.BookingSession, error) {
	var session model.BookingSession
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get booking session: %w", err)
	}
	return &session, nil
}

// UpdateSessionWithVersion performs the optimistic-lock update: it only
// applies if the row still has expectedVersion, and reports whether it did.
func (r *Repository) UpdateSessionWithVersion(ctx context.Context, session model.BookingSession, expectedVersion int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.BookingSession{}).
		Where("id = ? AND version = ?", session.ID, expectedVersion).
		Updates(map[string]any{
			"status":          session.Status,
			"service_id":      session.ServiceID,
			"professional_id": session.ProfessionalID,
			"slot_start_at":   session.SlotStartAt,
			"slot_end_at":     session.SlotEndAt,
			"slot_time_zone":  session.SlotTimeZone,
			"patient_name":    session.PatientName,
			"patient_email":   session.PatientEmail,
			"version":         expectedVersion + 1,
			"updated_at":      session.UpdatedAt,
		})
	if result.Error != nil {
		return false, fmt.Errorf("update booking session: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *Repository) WithTx(ctx context.Context, fn func(bookingsvc.TxRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txRepository{db: tx})
	})
}

// txRepository is pure CRUD scoped to one ConfirmAppointment transaction —
// no business rules; those live in internal/service/booking.
type txRepository struct {
	db *gorm.DB
}

var _ bookingsvc.TxRepository = (*txRepository)(nil)

func (t *txRepository) GetSessionForUpdate(ctx context.Context, id string) (*model.BookingSession, error) {
	var session model.BookingSession
	err := t.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session for update: %w", err)
	}
	return &session, nil
}

func (t *txRepository) UpdateSessionStatus(ctx context.Context, id, newStatus string, expectedVersion, newVersion int64) (bool, error) {
	result := t.db.WithContext(ctx).Model(&model.BookingSession{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Updates(map[string]any{"status": newStatus, "version": newVersion})
	if result.Error != nil {
		return false, fmt.Errorf("update session status: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (t *txRepository) FindIdempotencyKey(ctx context.Context, key string) (*model.AppointmentIdempotencyKey, error) {
	var rec model.AppointmentIdempotencyKey
	err := t.db.WithContext(ctx).Where("key = ?", key).Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find idempotency key: %w", err)
	}
	return &rec, nil
}

func (t *txRepository) InsertIdempotencyKey(ctx context.Context, rec model.AppointmentIdempotencyKey) error {
	if err := t.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func (t *txRepository) InsertAppointment(ctx context.Context, appt model.Appointment) error {
	if err := t.db.WithContext(ctx).Create(&appt).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresExclusionViolation {
			return bookingsvc.ErrSlotNoLongerAvailable
		}
		return fmt.Errorf("insert appointment: %w", err)
	}
	return nil
}

func (t *txRepository) InsertAuditLog(ctx context.Context, rec model.AppointmentAuditLog) error {
	if err := t.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}
