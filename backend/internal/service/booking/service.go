// Package booking implements the BookingSession lifecycle and Appointment
// confirmation described in backend/README.md's "PostgreSQL-first 預約一致性"
// and "防止重疊與狀態" sections. Outbox/Calendar delivery is intentionally
// not implemented yet (see README's "Outbox／Calendar delivery" note).
package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
	schedulingsvc "backend/internal/service/scheduling"
)

var (
	ErrSessionNotFound        = errors.New("booking: session not found")
	ErrSessionExpired         = errors.New("booking: session expired")
	ErrSessionVersionMismatch = errors.New("booking: session version mismatch")
	ErrInvalidStateTransition = errors.New("booking: invalid state transition")
	ErrValidationFailed       = errors.New("booking: missing or invalid fields for requested operation")
	ErrSlotNoLongerAvailable  = errors.New("booking: slot no longer available")
	ErrIdempotencyKeyReused   = errors.New("booking: idempotency key reused with a different request")
)

// SelectedSlot is the patient-facing shape of a chosen appointment slot.
type SelectedSlot struct {
	ProfessionalID string
	Start          time.Time
	End            time.Time
	TimeZone       string
}

// SessionPatch carries only the fields present in a PATCH request; nil
// means "leave unchanged".
type SessionPatch struct {
	ServiceCode     *string
	SelectedSlot    *SelectedSlot
	PatientName     *string
	PatientEmail    *string
	RequestedStatus *string
}

// TxRepository is the set of operations available within a single
// ConfirmAppointment transaction. Implementations must contain no business
// rules — all decisions live in Service.ConfirmAppointment.
type TxRepository interface {
	GetSessionForUpdate(ctx context.Context, id string) (*model.BookingSession, error)
	UpdateSessionStatus(ctx context.Context, id, newStatus string, expectedVersion, newVersion int64) (bool, error)
	FindIdempotencyKey(ctx context.Context, key string) (*model.AppointmentIdempotencyKey, error)
	InsertIdempotencyKey(ctx context.Context, rec model.AppointmentIdempotencyKey) error
	InsertAppointment(ctx context.Context, appt model.Appointment) error
	InsertAuditLog(ctx context.Context, rec model.AppointmentAuditLog) error
}

// Repository provides both the simple session CRUD used by the
// booking-sessions endpoints and the transaction boundary ConfirmAppointment
// needs, per README's "single PostgreSQL transaction" rule (see
// internal/service/seed for the same WithTx pattern).
type Repository interface {
	CreateSession(ctx context.Context, session model.BookingSession) error
	GetSession(ctx context.Context, id string) (*model.BookingSession, error)
	UpdateSessionWithVersion(ctx context.Context, session model.BookingSession, expectedVersion int64) (bool, error)
	WithTx(ctx context.Context, fn func(TxRepository) error) error
}

// IDGenerator is the injectable, CSPRNG-based UUID generator required by
// README's Canonical Types.
type IDGenerator interface {
	NewID() (string, error)
}

type Service struct {
	repo       Repository
	catalog    *catalogsvc.Service
	scheduling *schedulingsvc.Service
	ids        IDGenerator
	location   *time.Location
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(
	repo Repository,
	catalog *catalogsvc.Service,
	scheduling *schedulingsvc.Service,
	ids IDGenerator,
	location *time.Location,
	sessionTTL time.Duration,
) *Service {
	return &Service{
		repo:       repo,
		catalog:    catalog,
		scheduling: scheduling,
		ids:        ids,
		location:   location,
		sessionTTL: sessionTTL,
		now:        time.Now,
	}
}

func (s *Service) CreateSession(ctx context.Context) (*model.BookingSession, error) {
	id, err := s.ids.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}
	now := s.now()
	session := model.BookingSession{
		ID:        id,
		Status:    model.BookingSessionStatusCollecting,
		Version:   1,
		ExpiresAt: now.Add(s.sessionTTL),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session, nil
}

// GetSession returns the session, treating an expired session the same as
// a not-yet-terminal session past its expires_at: callers must never see
// stale collecting/readyToConfirm state as still actionable.
func (s *Service) GetSession(ctx context.Context, id string) (*model.BookingSession, error) {
	session, err := s.repo.GetSession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	if s.isExpired(session) {
		return nil, ErrSessionExpired
	}
	return session, nil
}

func (s *Service) isExpired(session *model.BookingSession) bool {
	if session.Status == model.BookingSessionStatusConfirmed || session.Status == model.BookingSessionStatusExpired {
		return session.Status == model.BookingSessionStatusExpired
	}
	return !session.ExpiresAt.After(s.now())
}

var validTransitions = map[string]map[string]bool{
	model.BookingSessionStatusCollecting:     {model.BookingSessionStatusReadyToConfirm: true, model.BookingSessionStatusExpired: true},
	model.BookingSessionStatusReadyToConfirm: {model.BookingSessionStatusCollecting: true, model.BookingSessionStatusConfirmed: true, model.BookingSessionStatusExpired: true},
}

// UpdateSession applies patch to the session identified by id, enforcing
// the optimistic-lock version, the BookingSession state machine, and (when
// transitioning into readyToConfirm) that all required fields are present
// and the selected slot is still within Scheduling's current availability.
func (s *Service) UpdateSession(ctx context.Context, id string, ifMatchVersion int64, patch SessionPatch) (*model.BookingSession, error) {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if session.Version != ifMatchVersion {
		return nil, ErrSessionVersionMismatch
	}
	if session.Status != model.BookingSessionStatusCollecting && session.Status != model.BookingSessionStatusReadyToConfirm {
		return nil, ErrInvalidStateTransition
	}

	updated := *session

	if patch.ServiceCode != nil {
		svc, err := s.findActiveServiceByCode(ctx, *patch.ServiceCode)
		if err != nil {
			return nil, err
		}
		if svc == nil {
			return nil, ErrValidationFailed
		}
		updated.ServiceID = &svc.ID
		// Changing the service invalidates any previously selected slot.
		if patch.SelectedSlot == nil {
			updated.ProfessionalID, updated.SlotStartAt, updated.SlotEndAt, updated.SlotTimeZone = nil, nil, nil, nil
		}
	}

	if patch.SelectedSlot != nil {
		if updated.ServiceID == nil {
			return nil, ErrValidationFailed
		}
		professionalID := patch.SelectedSlot.ProfessionalID
		start := patch.SelectedSlot.Start
		end := patch.SelectedSlot.End
		timeZone := patch.SelectedSlot.TimeZone
		updated.ProfessionalID = &professionalID
		updated.SlotStartAt = &start
		updated.SlotEndAt = &end
		updated.SlotTimeZone = &timeZone
	}

	if patch.PatientName != nil {
		name := strings.TrimSpace(*patch.PatientName)
		if len([]rune(name)) < 1 || len([]rune(name)) > 100 {
			return nil, ErrValidationFailed
		}
		updated.PatientName = &name
	}

	if patch.PatientEmail != nil {
		email := strings.TrimSpace(*patch.PatientEmail)
		if len(email) < 1 || len(email) > 254 {
			return nil, ErrValidationFailed
		}
		updated.PatientEmail = &email
	}

	targetStatus := session.Status
	if patch.RequestedStatus != nil {
		requested := *patch.RequestedStatus
		if !validTransitions[session.Status][requested] {
			return nil, ErrInvalidStateTransition
		}
		if requested == model.BookingSessionStatusReadyToConfirm {
			if err := s.requireReadyToConfirmFields(ctx, &updated); err != nil {
				return nil, err
			}
		}
		targetStatus = requested
	}

	updated.Status = targetStatus
	updated.UpdatedAt = s.now()

	applied, err := s.repo.UpdateSessionWithVersion(ctx, updated, session.Version)
	if err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	if !applied {
		return nil, ErrSessionVersionMismatch
	}
	updated.Version = session.Version + 1
	return &updated, nil
}

func (s *Service) requireReadyToConfirmFields(ctx context.Context, session *model.BookingSession) error {
	if session.ServiceID == nil || session.ProfessionalID == nil ||
		session.SlotStartAt == nil || session.SlotEndAt == nil ||
		session.PatientName == nil || session.PatientEmail == nil {
		return ErrValidationFailed
	}
	available, err := s.slotStillAvailable(ctx, *session.ServiceID, *session.ProfessionalID, *session.SlotStartAt, *session.SlotEndAt)
	if err != nil {
		return err
	}
	if !available {
		return ErrSlotNoLongerAvailable
	}
	return nil
}

func (s *Service) findActiveServiceByCode(ctx context.Context, code string) (*model.Service, error) {
	services, err := s.catalog.ListActiveServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active services: %w", err)
	}
	for _, svc := range services {
		if svc.Code == code {
			return &svc, nil
		}
	}
	return nil, nil
}

func (s *Service) slotStillAvailable(ctx context.Context, serviceID, professionalID string, start, end time.Time) (bool, error) {
	services, err := s.catalog.ListActiveServices(ctx)
	if err != nil {
		return false, fmt.Errorf("list active services: %w", err)
	}
	var serviceCode string
	found := false
	for _, svc := range services {
		if svc.ID == serviceID {
			serviceCode = svc.Code
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}

	date := start.In(s.location).Format("2006-01-02")
	slots, err := s.scheduling.GetAvailability(ctx, serviceCode, date)
	if err != nil {
		return false, fmt.Errorf("check availability: %w", err)
	}
	for _, slot := range slots {
		if slot.ProfessionalID == professionalID && slot.Start.Equal(start) && slot.End.Equal(end) {
			return true, nil
		}
	}
	return false, nil
}

type appointmentSnapshot struct {
	ID               string    `json:"id"`
	BookingSessionID string    `json:"bookingSessionId"`
	ServiceID        string    `json:"serviceId"`
	ProfessionalID   string    `json:"professionalId"`
	PatientName      string    `json:"patientName"`
	PatientEmail     string    `json:"patientEmail"`
	StartAt          time.Time `json:"startAt"`
	EndAt            time.Time `json:"endAt"`
	TimeZone         string    `json:"timeZone"`
}

func toSnapshot(appt model.Appointment) appointmentSnapshot {
	return appointmentSnapshot{
		ID: appt.ID, BookingSessionID: appt.BookingSessionID, ServiceID: appt.ServiceID,
		ProfessionalID: appt.ProfessionalID, PatientName: appt.PatientName, PatientEmail: appt.PatientEmail,
		StartAt: appt.StartAt, EndAt: appt.EndAt, TimeZone: appt.TimeZone,
	}
}

func fromSnapshot(s appointmentSnapshot) model.Appointment {
	return model.Appointment{
		ID: s.ID, BookingSessionID: s.BookingSessionID, ServiceID: s.ServiceID,
		ProfessionalID: s.ProfessionalID, PatientName: s.PatientName, PatientEmail: s.PatientEmail,
		StartAt: s.StartAt, EndAt: s.EndAt, TimeZone: s.TimeZone, Status: model.AppointmentStatusConfirmed,
	}
}

// ConfirmAppointment implements README's "PostgreSQL-first 預約一致性"
// steps 1-4 for the PostgreSQL-only portion of appointment confirmation
// (steps 5-6, the outbox delivery worker, are not implemented yet).
func (s *Service) ConfirmAppointment(ctx context.Context, sessionID string, ifMatchVersion int64, idempotencyKey, requestHash string) (*model.Appointment, error) {
	var result model.Appointment

	err := s.repo.WithTx(ctx, func(tx TxRepository) error {
		existing, err := tx.FindIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return fmt.Errorf("find idempotency key: %w", err)
		}
		if existing != nil {
			if existing.RequestHash != requestHash {
				return ErrIdempotencyKeyReused
			}
			var snapshot appointmentSnapshot
			if err := json.Unmarshal(existing.ResponseBody, &snapshot); err != nil {
				return fmt.Errorf("decode replayed appointment: %w", err)
			}
			result = fromSnapshot(snapshot)
			return nil
		}

		session, err := tx.GetSessionForUpdate(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("get session for update: %w", err)
		}
		if session == nil {
			return ErrSessionNotFound
		}
		if !session.ExpiresAt.After(s.now()) {
			return ErrSessionExpired
		}
		if session.Version != ifMatchVersion {
			return ErrSessionVersionMismatch
		}
		if session.Status != model.BookingSessionStatusReadyToConfirm {
			return ErrInvalidStateTransition
		}
		if session.ServiceID == nil || session.ProfessionalID == nil ||
			session.SlotStartAt == nil || session.SlotEndAt == nil ||
			session.PatientName == nil || session.PatientEmail == nil {
			return ErrValidationFailed
		}

		available, err := s.slotStillAvailable(ctx, *session.ServiceID, *session.ProfessionalID, *session.SlotStartAt, *session.SlotEndAt)
		if err != nil {
			return err
		}
		if !available {
			return ErrSlotNoLongerAvailable
		}

		apptID, err := s.ids.NewID()
		if err != nil {
			return fmt.Errorf("generate appointment id: %w", err)
		}
		now := s.now()
		timeZone := ""
		if session.SlotTimeZone != nil {
			timeZone = *session.SlotTimeZone
		}
		appt := model.Appointment{
			ID:               apptID,
			BookingSessionID: session.ID,
			ServiceID:        *session.ServiceID,
			ProfessionalID:   *session.ProfessionalID,
			PatientName:      *session.PatientName,
			PatientEmail:     *session.PatientEmail,
			StartAt:          *session.SlotStartAt,
			EndAt:            *session.SlotEndAt,
			TimeZone:         timeZone,
			Status:           model.AppointmentStatusConfirmed,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := tx.InsertAppointment(ctx, appt); err != nil {
			if errors.Is(err, ErrSlotNoLongerAvailable) {
				return ErrSlotNoLongerAvailable
			}
			return fmt.Errorf("insert appointment: %w", err)
		}

		auditID, err := s.ids.NewID()
		if err != nil {
			return fmt.Errorf("generate audit id: %w", err)
		}
		if err := tx.InsertAuditLog(ctx, model.AppointmentAuditLog{
			ID: auditID, EntityID: appt.ID, Action: "appointment.confirmed", ActorID: "system", CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}

		responseBody, err := json.Marshal(toSnapshot(appt))
		if err != nil {
			return fmt.Errorf("encode idempotency response: %w", err)
		}
		if err := tx.InsertIdempotencyKey(ctx, model.AppointmentIdempotencyKey{
			Key: idempotencyKey, RequestHash: requestHash, AppointmentID: &appt.ID,
			ResponseStatus: 201, ResponseBody: responseBody, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("insert idempotency key: %w", err)
		}

		applied, err := tx.UpdateSessionStatus(ctx, session.ID, model.BookingSessionStatusConfirmed, session.Version, session.Version+1)
		if err != nil {
			return fmt.Errorf("update session status: %w", err)
		}
		if !applied {
			return ErrSessionVersionMismatch
		}

		result = appt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
