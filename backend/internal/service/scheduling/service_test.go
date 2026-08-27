package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
)

type fakeCatalogServiceRepo struct {
	services []model.Service
}

func (f *fakeCatalogServiceRepo) ListActiveServices(ctx context.Context) ([]model.Service, error) {
	return f.services, nil
}

type fakeCatalogProfessionalRepo struct {
	professionals []model.Professional
}

func (f *fakeCatalogProfessionalRepo) ListActiveProfessionalsByServiceCode(ctx context.Context, serviceCode string) ([]model.Professional, error) {
	if serviceCode != "A" {
		return nil, nil
	}
	return f.professionals, nil
}

type fakeClinicScheduleRepo struct {
	hours   map[int16]*model.ClinicHours
	closure map[string]bool
}

func (f *fakeClinicScheduleRepo) GetHoursForDay(ctx context.Context, dayOfWeek int16) (*model.ClinicHours, error) {
	return f.hours[dayOfWeek], nil
}

func (f *fakeClinicScheduleRepo) IsClosureDate(ctx context.Context, date string) (bool, error) {
	return f.closure[date], nil
}

type fakeBlockedSlotRepo struct {
	slots []model.ProfessionalBlockedSlot
}

func (f *fakeBlockedSlotRepo) ListOverlapping(ctx context.Context, professionalID string, from, to time.Time) ([]model.ProfessionalBlockedSlot, error) {
	return f.slots, nil
}

type fakeConfirmedAppointmentRepo struct {
	appointments []model.Appointment
}

func (f *fakeConfirmedAppointmentRepo) ListOverlappingAppointments(ctx context.Context, professionalID string, from, to time.Time) ([]model.Appointment, error) {
	return f.appointments, nil
}

func openTime(hh, mm string) *string {
	s := hh + ":" + mm + ":00"
	return &s
}

func newTestService(t *testing.T, hoursRepo *fakeClinicScheduleRepo, blocked *fakeBlockedSlotRepo, appointments *fakeConfirmedAppointmentRepo) *Service {
	t.Helper()
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	catalog := catalogsvc.NewService(
		&fakeCatalogServiceRepo{services: []model.Service{{Code: "A", DurationMinutes: 60}}},
		&fakeCatalogProfessionalRepo{professionals: []model.Professional{{ID: "prof-1", Code: "SENIOR_1"}}},
	)
	svc := NewService(catalog, hoursRepo, blocked, appointments, loc, 30*time.Minute, 0)
	svc.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, loc) }
	return svc
}

func TestGetAvailability_NoQualifiedProfessionals(t *testing.T) {
	svc := newTestService(t,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{})

	slots, err := svc.GetAvailability(context.Background(), "UNQUALIFIED", "2026-09-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("expected no slots, got %+v", slots)
	}
}

func TestGetAvailability_InvalidDate(t *testing.T) {
	svc := newTestService(t, &fakeClinicScheduleRepo{}, &fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{})

	_, err := svc.GetAvailability(context.Background(), "A", "not-a-date")
	if !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("expected ErrInvalidDate, got %v", err)
	}
}

func TestGetAvailability_ClosureDate(t *testing.T) {
	svc := newTestService(t,
		&fakeClinicScheduleRepo{closure: map[string]bool{"2026-09-02": true}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{})

	slots, err := svc.GetAvailability(context.Background(), "A", "2026-09-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("expected no slots on closure date, got %+v", slots)
	}
}

func TestGetAvailability_DayNotOpen(t *testing.T) {
	// 2026-09-02 is a Wednesday (weekday 3); no entry configured for it.
	svc := newTestService(t,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{})

	slots, err := svc.GetAvailability(context.Background(), "A", "2026-09-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("expected no slots when clinic_hours missing for the weekday, got %+v", slots)
	}
}

func TestGetAvailability_HappyPath(t *testing.T) {
	weekday := int16(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC).Weekday())
	svc := newTestService(t,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{
			weekday: {DayOfWeek: weekday, IsOpen: true, OpenTime: openTime("09", "00"), CloseTime: openTime("11", "00")},
		}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{})

	slots, err := svc.GetAvailability(context.Background(), "A", "2026-09-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 09:00-11:00 window, 60-minute service, 30-minute interval -> 09:00 and 09:30 fit (09:30+60=10:30<=11:00); 10:00 fits too (10:00+60=11:00<=11:00).
	if len(slots) != 3 {
		t.Fatalf("expected 3 candidate slots, got %d: %+v", len(slots), slots)
	}
	for _, s := range slots {
		if s.ProfessionalID != "prof-1" {
			t.Fatalf("unexpected professional id: %+v", s)
		}
	}
}

func TestGetAvailability_ExcludesBlockedAndConfirmed(t *testing.T) {
	weekday := int16(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC).Weekday())
	loc := time.UTC
	svc := newTestService(t,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{
			weekday: {DayOfWeek: weekday, IsOpen: true, OpenTime: openTime("09", "00"), CloseTime: openTime("11", "00")},
		}},
		&fakeBlockedSlotRepo{slots: []model.ProfessionalBlockedSlot{
			{StartAt: time.Date(2026, 9, 2, 9, 0, 0, 0, loc), EndAt: time.Date(2026, 9, 2, 9, 30, 0, 0, loc)},
		}},
		&fakeConfirmedAppointmentRepo{appointments: []model.Appointment{
			{StartAt: time.Date(2026, 9, 2, 10, 30, 0, 0, loc), EndAt: time.Date(2026, 9, 2, 11, 0, 0, 0, loc), Status: model.AppointmentStatusConfirmed},
		}})

	slots, err := svc.GetAvailability(context.Background(), "A", "2026-09-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 09:00 overlaps blocked slot, 10:00 overlaps confirmed appointment; only 09:30 remains.
	if len(slots) != 1 || !slots[0].Start.Equal(time.Date(2026, 9, 2, 9, 30, 0, 0, loc)) {
		t.Fatalf("expected exactly the 09:30 slot, got %+v", slots)
	}
}
