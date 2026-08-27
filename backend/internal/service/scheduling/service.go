// Package scheduling computes anonymous availability slots from
// PostgreSQL-only inputs (clinic hours, closures, internal blocked slots,
// confirmed appointments and Catalog qualifications), per
// backend/README.md's PostgreSQL-first availability rule. It never reads
// Google Calendar or Microsoft Outlook.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
)

// ErrInvalidDate signals a date query parameter that is not a valid
// YYYY-MM-DD value. Handlers map this to 400 INVALID_REQUEST.
var ErrInvalidDate = errors.New("date does not match required format")

// Slot is an anonymous candidate appointment slot for one qualified
// professional.
type Slot struct {
	ProfessionalID string
	Start          time.Time
	End            time.Time
}

// ClinicScheduleRepository is implemented by internal/repository/scheduling.
type ClinicScheduleRepository interface {
	GetHoursForDay(ctx context.Context, dayOfWeek int16) (*model.ClinicHours, error)
	IsClosureDate(ctx context.Context, date string) (bool, error)
}

// BlockedSlotRepository is implemented by internal/repository/scheduling.
type BlockedSlotRepository interface {
	ListOverlapping(ctx context.Context, professionalID string, from, to time.Time) ([]model.ProfessionalBlockedSlot, error)
}

// ConfirmedAppointmentRepository is implemented by internal/repository/scheduling.
type ConfirmedAppointmentRepository interface {
	ListOverlappingAppointments(ctx context.Context, professionalID string, from, to time.Time) ([]model.Appointment, error)
}

type Service struct {
	catalog      *catalogsvc.Service
	hours        ClinicScheduleRepository
	blocked      BlockedSlotRepository
	appointments ConfirmedAppointmentRepository
	location     *time.Location
	slotInterval time.Duration
	minLead      time.Duration
	now          func() time.Time
}

func NewService(
	catalog *catalogsvc.Service,
	hours ClinicScheduleRepository,
	blocked BlockedSlotRepository,
	appointments ConfirmedAppointmentRepository,
	location *time.Location,
	slotInterval time.Duration,
	minLead time.Duration,
) *Service {
	return &Service{
		catalog:      catalog,
		hours:        hours,
		blocked:      blocked,
		appointments: appointments,
		location:     location,
		slotInterval: slotInterval,
		minLead:      minLead,
		now:          time.Now,
	}
}

// GetAvailability returns candidate slots for serviceCode on date
// (YYYY-MM-DD, interpreted in the clinic timezone). An empty result is a
// valid outcome (no qualified professional, closed day, fully booked, or
// clinic hours not yet configured) and must never be treated as an error.
func (s *Service) GetAvailability(ctx context.Context, serviceCode, date string) ([]Slot, error) {
	day, err := time.ParseInLocation("2006-01-02", date, s.location)
	if err != nil {
		return nil, ErrInvalidDate
	}

	professionals, err := s.catalog.ListQualifiedProfessionals(ctx, serviceCode)
	if err != nil {
		return nil, err
	}
	if len(professionals) == 0 {
		return []Slot{}, nil
	}

	services, err := s.catalog.ListActiveServices(ctx)
	if err != nil {
		return nil, err
	}
	var durationMinutes int16
	found := false
	for _, svc := range services {
		if svc.Code == serviceCode {
			durationMinutes = svc.DurationMinutes
			found = true
			break
		}
	}
	if !found {
		return []Slot{}, nil
	}
	duration := time.Duration(durationMinutes) * time.Minute

	closed, err := s.hours.IsClosureDate(ctx, day.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("check clinic closure: %w", err)
	}
	if closed {
		return []Slot{}, nil
	}

	weekday := int16(day.Weekday())
	dayHours, err := s.hours.GetHoursForDay(ctx, weekday)
	if err != nil {
		return nil, fmt.Errorf("get clinic hours: %w", err)
	}
	if dayHours == nil || !dayHours.IsOpen || dayHours.OpenTime == nil || dayHours.CloseTime == nil {
		return []Slot{}, nil
	}

	dayOpen, dayClose, err := combineDayWithTimeRange(day, *dayHours.OpenTime, *dayHours.CloseTime, s.location)
	if err != nil {
		return nil, fmt.Errorf("parse clinic hours: %w", err)
	}

	earliestStart := dayOpen
	if lead := s.now().Add(s.minLead); lead.After(earliestStart) {
		earliestStart = roundUpToInterval(lead, s.slotInterval)
	}

	slots := make([]Slot, 0)
	for _, professional := range professionals {
		blocked, err := s.blocked.ListOverlapping(ctx, professional.ID, dayOpen, dayClose)
		if err != nil {
			return nil, fmt.Errorf("list blocked slots: %w", err)
		}
		confirmed, err := s.appointments.ListOverlappingAppointments(ctx, professional.ID, dayOpen, dayClose)
		if err != nil {
			return nil, fmt.Errorf("list confirmed appointments: %w", err)
		}

		for start := earliestStart; !start.Add(duration).After(dayClose); start = start.Add(s.slotInterval) {
			end := start.Add(duration)
			if overlapsAny(start, end, blocked) || overlapsAppointments(start, end, confirmed) {
				continue
			}
			slots = append(slots, Slot{ProfessionalID: professional.ID, Start: start, End: end})
		}
	}

	return slots, nil
}

func combineDayWithTimeRange(day time.Time, openTime, closeTime string, loc *time.Location) (time.Time, time.Time, error) {
	open, err := time.ParseInLocation("15:04:05", openTime, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse open_time: %w", err)
	}
	close, err := time.ParseInLocation("15:04:05", closeTime, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse close_time: %w", err)
	}
	y, m, d := day.Date()
	dayOpen := time.Date(y, m, d, open.Hour(), open.Minute(), open.Second(), 0, loc)
	dayClose := time.Date(y, m, d, close.Hour(), close.Minute(), close.Second(), 0, loc)
	return dayOpen, dayClose, nil
}

func roundUpToInterval(t time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		return t
	}
	rem := t.Sub(t.Truncate(interval))
	if rem == 0 {
		return t
	}
	return t.Truncate(interval).Add(interval)
}

func overlapsAny(start, end time.Time, blocked []model.ProfessionalBlockedSlot) bool {
	for _, b := range blocked {
		if start.Before(b.EndAt) && b.StartAt.Before(end) {
			return true
		}
	}
	return false
}

func overlapsAppointments(start, end time.Time, appointments []model.Appointment) bool {
	for _, a := range appointments {
		if start.Before(a.EndAt) && a.StartAt.Before(end) {
			return true
		}
	}
	return false
}
