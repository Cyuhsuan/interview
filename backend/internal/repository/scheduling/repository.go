// Package scheduling is the only place in the Scheduling module allowed to
// import GORM or hold a *gorm.DB reference, per backend/README.md's
// tech-stack boundaries. All queries here are read-only.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/model"
	schedulingsvc "backend/internal/service/scheduling"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

var (
	_ schedulingsvc.ClinicScheduleRepository       = (*Repository)(nil)
	_ schedulingsvc.BlockedSlotRepository          = (*Repository)(nil)
	_ schedulingsvc.ConfirmedAppointmentRepository = (*Repository)(nil)
)

func (r *Repository) GetHoursForDay(ctx context.Context, dayOfWeek int16) (*model.ClinicHours, error) {
	var hours model.ClinicHours
	err := r.db.WithContext(ctx).Where("day_of_week = ?", dayOfWeek).Take(&hours).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get clinic hours for day: %w", err)
	}
	return &hours, nil
}

func (r *Repository) IsClosureDate(ctx context.Context, date string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ClinicClosure{}).
		Where("closure_date = ?", date).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check clinic closure: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) ListOverlapping(ctx context.Context, professionalID string, from, to time.Time) ([]model.ProfessionalBlockedSlot, error) {
	var slots []model.ProfessionalBlockedSlot
	err := r.db.WithContext(ctx).
		Where("professional_id = ?", professionalID).
		Where("start_at < ? AND end_at > ?", to, from).
		Find(&slots).Error
	if err != nil {
		return nil, fmt.Errorf("list blocked slots: %w", err)
	}
	return slots, nil
}

func (r *Repository) ListOverlappingAppointments(ctx context.Context, professionalID string, from, to time.Time) ([]model.Appointment, error) {
	var appointments []model.Appointment
	err := r.db.WithContext(ctx).
		Where("professional_id = ?", professionalID).
		Where("status = ?", model.AppointmentStatusConfirmed).
		Where("start_at < ? AND end_at > ?", to, from).
		Find(&appointments).Error
	if err != nil {
		return nil, fmt.Errorf("list confirmed appointments: %w", err)
	}
	return appointments, nil
}
