package booking

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
	schedulingsvc "backend/internal/service/scheduling"
)

// --- catalog fakes (constructs a real *catalogsvc.Service) ---

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

// --- scheduling fakes (constructs a real *schedulingsvc.Service) ---

type fakeClinicScheduleRepo struct {
	hours map[int16]*model.ClinicHours
}

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

// --- booking repository fake ---

type fakeIDGenerator struct{ n int }

func (g *fakeIDGenerator) NewID() (string, error) {
	g.n++
	return "id-" + itoa(g.n), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

type fakeTx struct {
	sessions     map[string]model.BookingSession
	idempotency  map[string]model.AppointmentIdempotencyKey
	appointments map[string]model.Appointment
	audits       []model.AppointmentAuditLog
	slotTaken    bool
}

func (t *fakeTx) GetSessionForUpdate(ctx context.Context, id string) (*model.BookingSession, error) {
	s, ok := t.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := s
	return &cp, nil
}

func (t *fakeTx) UpdateSessionStatus(ctx context.Context, id, newStatus string, expectedVersion, newVersion int64) (bool, error) {
	s, ok := t.sessions[id]
	if !ok || s.Version != expectedVersion {
		return false, nil
	}
	s.Status = newStatus
	s.Version = newVersion
	t.sessions[id] = s
	return true, nil
}

func (t *fakeTx) FindIdempotencyKey(ctx context.Context, key string) (*model.AppointmentIdempotencyKey, error) {
	rec, ok := t.idempotency[key]
	if !ok {
		return nil, nil
	}
	cp := rec
	return &cp, nil
}

func (t *fakeTx) InsertIdempotencyKey(ctx context.Context, rec model.AppointmentIdempotencyKey) error {
	t.idempotency[rec.Key] = rec
	return nil
}

func (t *fakeTx) InsertAppointment(ctx context.Context, appt model.Appointment) error {
	if t.slotTaken {
		return ErrSlotNoLongerAvailable
	}
	t.appointments[appt.ID] = appt
	return nil
}

func (t *fakeTx) InsertAuditLog(ctx context.Context, rec model.AppointmentAuditLog) error {
	t.audits = append(t.audits, rec)
	return nil
}

type fakeRepo struct {
	sessions     map[string]model.BookingSession
	idempotency  map[string]model.AppointmentIdempotencyKey
	appointments map[string]model.Appointment
	audits       []model.AppointmentAuditLog
	slotTaken    bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		sessions:     map[string]model.BookingSession{},
		idempotency:  map[string]model.AppointmentIdempotencyKey{},
		appointments: map[string]model.Appointment{},
	}
}

func (r *fakeRepo) CreateSession(ctx context.Context, session model.BookingSession) error {
	r.sessions[session.ID] = session
	return nil
}

func (r *fakeRepo) GetSession(ctx context.Context, id string) (*model.BookingSession, error) {
	s, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := s
	return &cp, nil
}

func (r *fakeRepo) UpdateSessionWithVersion(ctx context.Context, session model.BookingSession, expectedVersion int64) (bool, error) {
	existing, ok := r.sessions[session.ID]
	if !ok || existing.Version != expectedVersion {
		return false, nil
	}
	session.Version = expectedVersion + 1
	r.sessions[session.ID] = session
	return true, nil
}

func (r *fakeRepo) WithTx(ctx context.Context, fn func(TxRepository) error) error {
	tx := &fakeTx{
		sessions:     cloneSessions(r.sessions),
		idempotency:  cloneIdempotency(r.idempotency),
		appointments: cloneAppointments(r.appointments),
		slotTaken:    r.slotTaken,
	}
	if err := fn(tx); err != nil {
		return err
	}
	r.sessions = tx.sessions
	r.idempotency = tx.idempotency
	r.appointments = tx.appointments
	r.audits = append(r.audits, tx.audits...)
	return nil
}

func cloneSessions(m map[string]model.BookingSession) map[string]model.BookingSession {
	out := make(map[string]model.BookingSession, len(m))
	maps.Copy(out, m)
	return out
}

func cloneIdempotency(m map[string]model.AppointmentIdempotencyKey) map[string]model.AppointmentIdempotencyKey {
	out := make(map[string]model.AppointmentIdempotencyKey, len(m))
	maps.Copy(out, m)
	return out
}

func cloneAppointments(m map[string]model.Appointment) map[string]model.Appointment {
	out := make(map[string]model.Appointment, len(m))
	maps.Copy(out, m)
	return out
}

// --- test fixture wiring ---

const testServiceID = "svc-a"
const testProfessionalID = "prof-1"

func newTestService(t *testing.T, repo *fakeRepo) (*Service, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	catalog := catalogsvc.NewService(
		&fakeCatalogServiceRepo{services: []model.Service{{ID: testServiceID, Code: "A", DurationMinutes: 60}}},
		&fakeCatalogProfessionalRepo{professionals: []model.Professional{{ID: testProfessionalID, Code: "SENIOR_1"}}},
	)

	weekday := int16(time.Date(2026, 9, 2, 0, 0, 0, 0, loc).Weekday())
	open := "09:00:00"
	close := "17:00:00"
	scheduling := schedulingsvc.NewService(
		catalog,
		&fakeClinicScheduleRepo{hours: map[int16]*model.ClinicHours{
			weekday: {DayOfWeek: weekday, IsOpen: true, OpenTime: &open, CloseTime: &close},
		}},
		&fakeBlockedSlotRepo{}, &fakeConfirmedAppointmentRepo{},
		loc, 30*time.Minute, 0,
	)

	svc := NewService(repo, catalog, scheduling, &fakeIDGenerator{}, loc, 15*time.Minute)
	svc.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, loc) }
	return svc, loc
}

func validSlot(loc *time.Location) (time.Time, time.Time) {
	start := time.Date(2026, 9, 2, 9, 0, 0, 0, loc)
	return start, start.Add(60 * time.Minute)
}

func TestCreateSession(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newTestService(t, repo)

	session, err := svc.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Status != model.BookingSessionStatusCollecting || session.Version != 1 {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newTestService(t, repo)

	_, err := svc.GetSession(context.Background(), "missing")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetSession_Expired(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ExpiresAt: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
	}

	_, err := svc.GetSession(context.Background(), "s1")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestUpdateSession_VersionMismatch(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	_, err := svc.UpdateSession(context.Background(), "s1", 99, SessionPatch{})
	if !errors.Is(err, ErrSessionVersionMismatch) {
		t.Fatalf("expected ErrSessionVersionMismatch, got %v", err)
	}
}

func TestUpdateSession_ReadyToConfirmRequiresFields(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	status := model.BookingSessionStatusReadyToConfirm
	_, err := svc.UpdateSession(context.Background(), "s1", 1, SessionPatch{RequestedStatus: &status})
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}
}

func TestUpdateSession_HappyPathToReadyToConfirm(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	start, end := validSlot(loc)
	code := "A"
	name := "Jane Doe"
	email := "jane@example.com"
	status := model.BookingSessionStatusReadyToConfirm
	patch := SessionPatch{
		ServiceCode:     &code,
		SelectedSlot:    &SelectedSlot{ProfessionalID: testProfessionalID, Start: start, End: end, TimeZone: "UTC"},
		PatientName:     &name,
		PatientEmail:    &email,
		RequestedStatus: &status,
	}

	session, err := svc.UpdateSession(context.Background(), "s1", 1, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Status != model.BookingSessionStatusReadyToConfirm || session.Version != 2 {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestUpdateSession_InvalidTransition(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	confirmed := model.BookingSessionStatusConfirmed
	_, err := svc.UpdateSession(context.Background(), "s1", 1, SessionPatch{RequestedStatus: &confirmed})
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func readySession(loc *time.Location) model.BookingSession {
	start, end := validSlot(loc)
	svcID, profID := testServiceID, testProfessionalID
	name, email := "Jane Doe", "jane@example.com"
	tz := "UTC"
	return model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusReadyToConfirm, Version: 2,
		ServiceID: &svcID, ProfessionalID: &profID,
		SlotStartAt: &start, SlotEndAt: &end, SlotTimeZone: &tz,
		PatientName: &name, PatientEmail: &email,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}
}

func TestConfirmAppointment_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = readySession(loc)

	appt, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appt.ProfessionalID != testProfessionalID || appt.Status != model.AppointmentStatusConfirmed {
		t.Fatalf("unexpected appointment: %+v", appt)
	}
	if repo.sessions["s1"].Status != model.BookingSessionStatusConfirmed {
		t.Fatalf("expected session to be confirmed, got %+v", repo.sessions["s1"])
	}
	if len(repo.audits) != 1 {
		t.Fatalf("expected one audit record, got %d", len(repo.audits))
	}
}

func TestConfirmAppointment_IdempotentReplay(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = readySession(loc)

	first, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1")
	if err != nil {
		t.Fatalf("unexpected error on replay: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected replay to return the same appointment: %+v vs %+v", first, second)
	}
}

func TestConfirmAppointment_IdempotencyKeyReused(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = readySession(loc)

	if _, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "different-hash")
	if !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("expected ErrIdempotencyKeyReused, got %v", err)
	}
}

func TestConfirmAppointment_WrongState(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	session := readySession(loc)
	session.Status = model.BookingSessionStatusCollecting
	repo.sessions["s1"] = session

	_, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1")
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestConfirmAppointment_SlotNoLongerAvailable(t *testing.T) {
	repo := newFakeRepo()
	repo.slotTaken = true
	svc, loc := newTestService(t, repo)
	repo.sessions["s1"] = readySession(loc)

	_, err := svc.ConfirmAppointment(context.Background(), "s1", 2, "idem-key-0123456789", "hash-1")
	if !errors.Is(err, ErrSlotNoLongerAvailable) {
		t.Fatalf("expected ErrSlotNoLongerAvailable, got %v", err)
	}
}

func TestUpdateSession_OfferedSlotsRoundTrip(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	svcID := testServiceID
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ServiceID: &svcID,
		ExpiresAt: time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	start, end := validSlot(loc)
	slots := []SelectedSlot{{ProfessionalID: testProfessionalID, Start: start, End: end, TimeZone: "UTC"}}

	updated, err := svc.UpdateSession(context.Background(), "s1", 1, SessionPatch{OfferedSlots: &slots})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded, err := DecodeOfferedSlots(updated.OfferedSlots)
	if err != nil {
		t.Fatalf("decode offered slots: %v", err)
	}
	if len(decoded) != 1 || decoded[0].ProfessionalID != testProfessionalID || !decoded[0].Start.Equal(start) {
		t.Fatalf("unexpected round-tripped slots: %+v", decoded)
	}
}

func TestUpdateSession_SelectingSlotClearsOfferedSlots(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	svcID := testServiceID
	start, end := validSlot(loc)
	stored, err := EncodeOfferedSlots([]SelectedSlot{{ProfessionalID: testProfessionalID, Start: start, End: end, TimeZone: "UTC"}})
	if err != nil {
		t.Fatalf("encode offered slots: %v", err)
	}
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ServiceID:    &svcID,
		OfferedSlots: stored,
		ExpiresAt:    time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	updated, err := svc.UpdateSession(context.Background(), "s1", 1, SessionPatch{
		SelectedSlot: &SelectedSlot{ProfessionalID: testProfessionalID, Start: start, End: end, TimeZone: "UTC"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.OfferedSlots != nil {
		t.Fatalf("expected OfferedSlots to be cleared once a slot is selected, got %v", updated.OfferedSlots)
	}
}

func TestUpdateSession_ChangingServiceClearsOfferedSlots(t *testing.T) {
	repo := newFakeRepo()
	svc, loc := newTestService(t, repo)
	svcID := testServiceID
	start, end := validSlot(loc)
	stored, err := EncodeOfferedSlots([]SelectedSlot{{ProfessionalID: testProfessionalID, Start: start, End: end, TimeZone: "UTC"}})
	if err != nil {
		t.Fatalf("encode offered slots: %v", err)
	}
	repo.sessions["s1"] = model.BookingSession{
		ID: "s1", Status: model.BookingSessionStatusCollecting, Version: 1,
		ServiceID:    &svcID,
		OfferedSlots: stored,
		ExpiresAt:    time.Date(2026, 9, 10, 0, 0, 0, 0, loc),
	}

	code := "A"
	updated, err := svc.UpdateSession(context.Background(), "s1", 1, SessionPatch{ServiceCode: &code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.OfferedSlots != nil {
		t.Fatalf("expected OfferedSlots to be cleared on a service change, got %v", updated.OfferedSlots)
	}
}

func TestEncodeDecodeOfferedSlots_EmptyAndRoundTrip(t *testing.T) {
	encoded, err := EncodeOfferedSlots(nil)
	if err != nil || encoded != nil {
		t.Fatalf("expected nil bytes for an empty slice, got %v, %v", encoded, err)
	}
	decoded, err := DecodeOfferedSlots(nil)
	if err != nil || decoded != nil {
		t.Fatalf("expected nil slice for nil bytes, got %v, %v", decoded, err)
	}

	start := time.Date(2026, 9, 2, 9, 30, 0, 0, time.UTC)
	end := start.Add(60 * time.Minute)
	original := []SelectedSlot{{ProfessionalID: "prof-1", Start: start, End: end, TimeZone: "UTC"}}

	data, err := EncodeOfferedSlots(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	roundTripped, err := DecodeOfferedSlots(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(roundTripped) != 1 || !roundTripped[0].Start.Equal(start) || !roundTripped[0].End.Equal(end) {
		t.Fatalf("round-trip mismatch: %+v", roundTripped)
	}
}
