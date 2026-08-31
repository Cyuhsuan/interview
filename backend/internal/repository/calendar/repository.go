// Package calendar is the only place allowed to import GORM or hold a
// *gorm.DB for the Calendar module, per backend/README.md's tech-stack
// boundaries.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/model"
	calendarsvc "backend/internal/service/calendar"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

var _ calendarsvc.OutboxRepository = (*Repository)(nil)

// LockAndMarkProcessing implements README step 5's first half: lock the
// next due row FOR UPDATE SKIP LOCKED and commit it as processing in one
// short transaction, separate from the external Calendar call that follows.
func (r *Repository) LockAndMarkProcessing(ctx context.Context) (*model.AppointmentOutbox, error) {
	var rec *model.AppointmentOutbox
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.AppointmentOutbox
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", []string{model.OutboxStatusPending, model.OutboxStatusRetryable}, time.Now()).
			Order("next_attempt_at").
			Limit(1).
			Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("select next due outbox row: %w", err)
		}

		result := tx.Model(&model.AppointmentOutbox{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{
				"status":        model.OutboxStatusProcessing,
				"attempt_count": row.AttemptCount + 1,
				"updated_at":    time.Now(),
			})
		if result.Error != nil {
			return fmt.Errorf("mark outbox row processing: %w", result.Error)
		}

		row.Status = model.OutboxStatusProcessing
		row.AttemptCount++
		rec = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Repository) MarkDelivered(ctx context.Context, id, eventReference string) error {
	result := r.db.WithContext(ctx).Model(&model.AppointmentOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":          model.OutboxStatusDelivered,
			"event_reference": eventReference,
			"last_error":      nil,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark outbox row delivered: %w", result.Error)
	}
	return nil
}

func (r *Repository) MarkRetryable(ctx context.Context, id string, nextAttemptAt time.Time, lastError string) error {
	result := r.db.WithContext(ctx).Model(&model.AppointmentOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":          model.OutboxStatusRetryable,
			"next_attempt_at": nextAttemptAt,
			"last_error":      lastError,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark outbox row retryable: %w", result.Error)
	}
	return nil
}

func (r *Repository) MarkDeadLetter(ctx context.Context, id string, lastError string) error {
	result := r.db.WithContext(ctx).Model(&model.AppointmentOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     model.OutboxStatusDeadLetter,
			"last_error": lastError,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark outbox row dead_letter: %w", result.Error)
	}
	return nil
}

func (r *Repository) GetAppointment(ctx context.Context, appointmentID string) (*model.Appointment, error) {
	var appt model.Appointment
	err := r.db.WithContext(ctx).Where("id = ?", appointmentID).Take(&appt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get appointment: %w", err)
	}
	return &appt, nil
}

func (r *Repository) ListForAppointment(ctx context.Context, appointmentID string) ([]model.AppointmentOutbox, error) {
	var rows []model.AppointmentOutbox
	if err := r.db.WithContext(ctx).Where("appointment_id = ?", appointmentID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list outbox rows for appointment: %w", err)
	}
	return rows, nil
}

func (r *Repository) ListDeadLetter(ctx context.Context) ([]model.AppointmentOutbox, error) {
	var rows []model.AppointmentOutbox
	if err := r.db.WithContext(ctx).Where("status = ?", model.OutboxStatusDeadLetter).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list dead_letter outbox rows: %w", err)
	}
	return rows, nil
}
