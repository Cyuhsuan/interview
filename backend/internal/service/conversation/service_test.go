package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/model"
	bookingsvc "backend/internal/service/booking"
	catalogsvc "backend/internal/service/catalog"
	schedulingsvc "backend/internal/service/scheduling"
)

// --- catalog fakes ---

type fakeCatalogServiceRepo struct{ services []model.Service }

func (f *fakeCatalogServiceRepo) ListActiveServices(ctx context.Context) ([]model.Service, error) {
	return f.services, nil
}

type fakeCatalogProfessionalRepo struct{ professionals []model.Professional }

func (f *fakeCatalogProfessionalRepo) ListActiveProfessionalsByServiceCode(ctx context.Context, serviceCode string) ([]model.Professional, error) {
	if serviceCode != "A" {
		return nil, nil
	}
	return f.professionals, nil
}

// --- scheduling fakes ---

type fakeClinicScheduleRepo struct{ hours map[int16]*model.ClinicHours }

func (f *fakeClinicScheduleRepo) GetHoursForDay(ctx context.Context, dayOfWeek int16) (*model.ClinicHours, error) {
	return f.hours[dayOfWeek], nil
}
func (f *fakeClinicScheduleRepo) IsClosureDate(ctx context.Context, date string) (bool, error) {
	return false, nil
}

type fakeBlockedSlotRepo struct{}

func (f *fakeBlockedSlotRepo) ListOverlapping(ctx context.Context, professionalID string, from, to time.Time) ([]model.ProfessionalBlockedSlot, error) {
	return nil, nil
}

type fakeConfirmedAppointmentRepo struct{}

func (f *fakeConfirmedAppointmentRepo) ListOverlappingAppointments(ctx context.Context, professionalID string, from, to time.Time) ([]model.Appointment, error) {
	return nil, nil
}

// --- booking repository fake (SendMessage only ever calls GetSession /
// UpdateSession, never ConfirmAppointment, so the TxRepository side is
// unused but still required to satisfy bookingsvc.Repository). ---

type fakeIDGenerator struct{ n int }

func (g *fakeIDGenerator) NewID() (string, error) {
	g.n++
	return "id", nil
}

type fakeTx struct{}

func (t *fakeTx) GetSessionForUpdate(ctx context.Context, id string) (*model.BookingSession, error) {
	return nil, nil
}
func (t *fakeTx) UpdateSessionStatus(ctx context.Context, id, newStatus string, expectedVersion, newVersion int64) (bool, error) {
	return false, nil
}
func (t *fakeTx) FindIdempotencyKey(ctx context.Context, key string) (*model.AppointmentIdempotencyKey, error) {
	return nil, nil
}
func (t *fakeTx) InsertIdempotencyKey(ctx context.Context, rec model.AppointmentIdempotencyKey) error {
	return nil
}
func (t *fakeTx) InsertAppointment(ctx context.Context, appt model.Appointment) error { return nil }
func (t *fakeTx) InsertAuditLog(ctx context.Context, rec model.AppointmentAuditLog) error {
	return nil
}

type fakeBookingRepo struct {
	sessions map[string]model.BookingSession
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{sessions: map[string]model.BookingSession{}}
}

func (r *fakeBookingRepo) CreateSession(ctx context.Context, session model.BookingSession) error {
	r.sessions[session.ID] = session
	return nil
}
func (r *fakeBookingRepo) GetSession(ctx context.Context, id string) (*model.BookingSession, error) {
	s, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := s
	return &cp, nil
}
func (r *fakeBookingRepo) UpdateSessionWithVersion(ctx context.Context, session model.BookingSession, expectedVersion int64) (bool, error) {
	existing, ok := r.sessions[session.ID]
	if !ok || existing.Version != expectedVersion {
		return false, nil
	}
	session.Version = expectedVersion + 1
	r.sessions[session.ID] = session
	return true, nil
}
func (r *fakeBookingRepo) WithTx(ctx context.Context, fn func(bookingsvc.TxRepository) error) error {
	return fn(&fakeTx{})
}

// --- fake AIProvider: a deterministic stand-in for the real internal/ai
// LLM client, driven entirely by test-supplied Extraction values. ---

type fakeAI struct {
	extraction Extraction
	err        error
}

func (f *fakeAI) Extract(ctx context.Context, message string, ref time.Time, knownServiceCodes []string) (Extraction, error) {
	return f.extraction, f.err
}

const testServiceID = "svc-a"
const testProfessionalID = "prof-1"

func newTestService(t *testing.T, bookingRepo *fakeBookingRepo, ai AIProvider) *Service {
	t.Helper()
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	catalog := catalogsvc.NewService(
		&fakeCatalogServiceRepo{services: []model.Service{{ID: testServiceID, Code: "A", DisplayName: "Service A", DurationMinutes: 60}}},
		&fakeCatalogProfessionalRepo{professionals: []model.Professional{{ID: testProfessionalID, Code: "SENIOR_1"}}},
	)

	weekday := int16(time.Date(2026, 9, 2, 0, 0, 0, 0, loc).Weekday())
	open := "09:00:00"
	closeTime := "17:00:00"
	scheduling := schedulingsvc.NewService(
		catalog,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{
			weekday: {DayOfWeek: weekday, IsOpen: true, OpenTime: &open, CloseTime: &closeTime},
		}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{},
		loc, 30*time.Minute, 0,
	)

	booking := bookingsvc.NewService(bookingRepo, catalog, scheduling, &fakeIDGenerator{}, loc, 15*time.Minute)

	return NewService(booking, scheduling, catalog, ai, loc)
}

func newCollectingSession(id string) model.BookingSession {
	return model.BookingSession{
		ID:        id,
		Status:    model.BookingSessionStatusCollecting,
		Version:   1,
		ExpiresAt: time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC),
	}
}

func TestSendMessage_OutOfScopeUsesFixedTemplateAndDoesNotMutateSession(t *testing.T) {
	repo := newFakeBookingRepo()
	repo.sessions["s1"] = newCollectingSession("s1")
	code := "A"
	ai := &fakeAI{extraction: Extraction{OutOfScopeCategory: "emergency", ServiceCode: &code}}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "I have a dental emergency, what should I do?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reply.OutOfScope {
		t.Fatalf("expected OutOfScope=true")
	}
	if reply.Text != outOfScopeReplies["emergency"] {
		t.Fatalf("expected fixed emergency template, got %q", reply.Text)
	}
	if reply.Session.ServiceID != nil {
		t.Fatalf("out-of-scope turn must not apply candidate fields, got ServiceID=%v", reply.Session.ServiceID)
	}
}

func TestSendMessage_AppliesValidServiceCodeCandidate(t *testing.T) {
	repo := newFakeBookingRepo()
	repo.sessions["s1"] = newCollectingSession("s1")
	code := "A"
	ai := &fakeAI{extraction: Extraction{ServiceCode: &code}}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "I'd like a Service A appointment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Session.ServiceID == nil || *reply.Session.ServiceID != testServiceID {
		t.Fatalf("expected serviceId to be applied, got %v", reply.Session.ServiceID)
	}
	if reply.OutOfScope {
		t.Fatalf("expected OutOfScope=false")
	}
}

func TestSendMessage_UnknownServiceCodeCandidateIsIgnoredNotAnError(t *testing.T) {
	repo := newFakeBookingRepo()
	repo.sessions["s1"] = newCollectingSession("s1")
	bogus := "ZZZ"
	ai := &fakeAI{extraction: Extraction{ServiceCode: &bogus}}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "book me the ZZZ thing")
	if err != nil {
		t.Fatalf("expected no error for an unrecognized candidate, got %v", err)
	}
	if reply.Session.ServiceID != nil {
		t.Fatalf("unknown service code must not be applied, got %v", reply.Session.ServiceID)
	}
	if reply.Text == "" {
		t.Fatalf("expected a clarifying reply")
	}
}

func TestSendMessage_NoDateYieldsNoOfferedSlots(t *testing.T) {
	repo := newFakeBookingRepo()
	session := newCollectingSession("s1")
	session.ServiceID = ptr(testServiceID)
	repo.sessions["s1"] = session
	ai := &fakeAI{extraction: Extraction{}}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply.OfferedSlots) != 0 {
		t.Fatalf("expected no offered slots without a resolved date, got %d", len(reply.OfferedSlots))
	}
}

func TestSendMessage_ServiceAndDateYieldsOfferedSlotsFromScheduling(t *testing.T) {
	repo := newFakeBookingRepo()
	session := newCollectingSession("s1")
	session.ServiceID = ptr(testServiceID)
	repo.sessions["s1"] = session
	date := "2026-09-02"
	ai := &fakeAI{extraction: Extraction{DateISO: &date}}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "how about Sept 2nd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply.OfferedSlots) == 0 {
		t.Fatalf("expected offered slots for an open clinic day, got none")
	}
}

func TestSendMessage_AIFailureFallsBackToClarifyingReplyWithoutError(t *testing.T) {
	repo := newFakeBookingRepo()
	repo.sessions["s1"] = newCollectingSession("s1")
	ai := &fakeAI{err: errors.New("provider timeout")}
	svc := newTestService(t, repo, ai)

	reply, err := svc.SendMessage(context.Background(), "s1", "anything")
	if err != nil {
		t.Fatalf("AI provider failure must not surface as a service error, got %v", err)
	}
	if reply.Text != clarifyReply {
		t.Fatalf("expected clarifyReply fallback, got %q", reply.Text)
	}
}

func TestSendMessage_ExpiredSessionPropagatesAsError(t *testing.T) {
	repo := newFakeBookingRepo()
	session := newCollectingSession("s1")
	session.ExpiresAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	repo.sessions["s1"] = session
	ai := &fakeAI{}
	svc := newTestService(t, repo, ai)

	_, err := svc.SendMessage(context.Background(), "s1", "hello")
	if !errors.Is(err, bookingsvc.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func ptr(s string) *string { return &s }
