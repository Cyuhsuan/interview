// Package conversation implements the single-turn chat flow described in
// backend/README.md's POST /booking-sessions/{id}/messages contract. It
// never decides booking legality itself: every candidate value the
// AIProvider extracts is re-validated by the existing, already-tested
// internal/service/booking and internal/service/scheduling deterministic
// logic before it can change a BookingSession, per README's "AI 只能理解
// 語言，但預約是否合法必須由確定性的後端程式碼判斷" rule.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"backend/internal/model"
	bookingsvc "backend/internal/service/booking"
	catalogsvc "backend/internal/service/catalog"
	schedulingsvc "backend/internal/service/scheduling"
)

// Extraction is the AI provider's best-effort, single-turn read of a
// patient message. Every field is a candidate — Service validates each one
// deterministically before applying it, and silently drops candidates that
// don't validate rather than surfacing an HTTP error, so a chat
// misunderstanding is never conflated with a real API error.
type Extraction struct {
	// OutOfScopeCategory is "" for in-scope messages, or one of
	// "diagnosis", "prescription", "emergency", "price", "insurance",
	// "cancel_reschedule".
	OutOfScopeCategory string
	ServiceCode        *string
	// DateISO is YYYY-MM-DD, resolved by the provider relative to the ref
	// time passed to Extract (the service never lets the AI use its own
	// notion of "now").
	DateISO      *string
	TimeOfDay    *string // "morning" | "afternoon" | "evening"
	PatientName  *string
	PatientEmail *string
}

// AIProvider is the provider-neutral port from backend/README.md's "AI
// Provider Adapter Contract". Implementations must only extract candidate
// values for one turn — no multi-turn memory, no slot selection, no
// booking-legality judgment.
type AIProvider interface {
	Extract(ctx context.Context, message string, ref time.Time, knownServiceCodes []string) (Extraction, error)
}

// Reply is what internal/handler/conversation renders as the messages
// endpoint's JSON response.
type Reply struct {
	Session      *model.BookingSession
	Text         string
	OfferedSlots []schedulingsvc.Slot
	OutOfScope   bool
}

// outOfScopeReplies are fixed, deterministic hand-off templates. The AI
// model is never allowed to generate this text itself, so a model glitch
// can't produce a diagnosis, a price quote, or a cancellation promise.
var outOfScopeReplies = map[string]string{
	"diagnosis":         "I'm not able to diagnose symptoms or give medical advice. Please contact the clinic directly to discuss this with a professional.",
	"prescription":      "I can't prescribe or advise on medication. Please contact the clinic directly for that.",
	"emergency":         "If this is a medical emergency, please seek emergency care right away. For non-emergency questions, please contact the clinic directly.",
	"price":             "I'm not able to provide pricing or quotes. Please contact the clinic directly for cost information.",
	"insurance":         "I can't help with insurance questions. Please contact the clinic directly for that.",
	"cancel_reschedule": "I'm not able to cancel or reschedule appointments here. Please contact the clinic directly for that.",
}

const clarifyReply = "Sorry, I didn't quite catch that. Could you rephrase, or tell me which service and date you'd like?"

const maxOfferedSlots = 5

type Service struct {
	booking    *bookingsvc.Service
	scheduling *schedulingsvc.Service
	catalog    *catalogsvc.Service
	ai         AIProvider
	location   *time.Location
	now        func() time.Time
}

func NewService(
	booking *bookingsvc.Service,
	scheduling *schedulingsvc.Service,
	catalog *catalogsvc.Service,
	ai AIProvider,
	location *time.Location,
) *Service {
	return &Service{
		booking:    booking,
		scheduling: scheduling,
		catalog:    catalog,
		ai:         ai,
		location:   location,
		now:        time.Now,
	}
}

// SendMessage implements one turn of the chat contract. Session-level
// errors (not found, expired) are returned as-is so the handler can map
// them to the same HTTP status as the other booking-sessions endpoints;
// everything else about "did the bot understand" is reflected in the
// returned Reply.Text, never as an error.
func (s *Service) SendMessage(ctx context.Context, sessionID, message string) (Reply, error) {
	session, err := s.booking.GetSession(ctx, sessionID)
	if err != nil {
		return Reply{}, err
	}

	services, err := s.catalog.ListActiveServices(ctx)
	if err != nil {
		return Reply{}, fmt.Errorf("list active services: %w", err)
	}
	knownCodes := make([]string, 0, len(services))
	for _, svc := range services {
		knownCodes = append(knownCodes, svc.Code)
	}

	extraction, err := s.ai.Extract(ctx, message, s.now(), knownCodes)
	if err != nil {
		return Reply{Session: session, Text: clarifyReply}, nil
	}

	if extraction.OutOfScopeCategory != "" {
		text, ok := outOfScopeReplies[extraction.OutOfScopeCategory]
		if !ok {
			text = clarifyReply
		}
		return Reply{Session: session, Text: text, OutOfScope: true}, nil
	}

	patch := bookingsvc.SessionPatch{}
	if extraction.ServiceCode != nil && codeKnown(knownCodes, *extraction.ServiceCode) {
		patch.ServiceCode = extraction.ServiceCode
	}
	if extraction.PatientName != nil {
		if name, ok := validPatientName(*extraction.PatientName); ok {
			patch.PatientName = &name
		}
	}
	if extraction.PatientEmail != nil {
		if email, ok := validPatientEmail(*extraction.PatientEmail); ok {
			patch.PatientEmail = &email
		}
	}

	updated := s.applyPatch(ctx, session, patch)

	var offered []schedulingsvc.Slot
	if updated.ServiceID != nil && extraction.DateISO != nil {
		if code := serviceCodeByID(services, *updated.ServiceID); code != "" {
			slots, err := s.scheduling.GetAvailability(ctx, code, *extraction.DateISO)
			if err != nil {
				return Reply{}, fmt.Errorf("get availability: %w", err)
			}
			offered = filterByTimeOfDay(slots, extraction.TimeOfDay, s.location)
			if len(offered) > maxOfferedSlots {
				offered = offered[:maxOfferedSlots]
			}
		}
	}

	text := s.buildReply(services, updated, offered, extraction.DateISO != nil)
	return Reply{Session: updated, Text: text, OfferedSlots: offered}, nil
}

// applyPatch applies patch through the existing, already-validated
// bookingsvc.UpdateSession logic. Any failure — a stale optimistic-lock
// version, a candidate that doesn't pass booking's own validation, an
// invalid state transition — is treated as "this turn's candidate values
// didn't apply," not as an error: the caller still gets a 200 with a
// clarifying reply. A version mismatch gets exactly one retry against a
// freshly re-read session, since chat has no client-visible If-Match to
// resolve it with.
func (s *Service) applyPatch(ctx context.Context, session *model.BookingSession, patch bookingsvc.SessionPatch) *model.BookingSession {
	if patch.ServiceCode == nil && patch.PatientName == nil && patch.PatientEmail == nil {
		return session
	}

	updated, err := s.booking.UpdateSession(ctx, session.ID, session.Version, patch)
	if err == nil {
		return updated
	}
	if !errors.Is(err, bookingsvc.ErrSessionVersionMismatch) {
		return session
	}

	fresh, ferr := s.booking.GetSession(ctx, session.ID)
	if ferr != nil {
		return session
	}
	retried, rerr := s.booking.UpdateSession(ctx, fresh.ID, fresh.Version, patch)
	if rerr != nil {
		return fresh
	}
	return retried
}

func (s *Service) buildReply(services []model.Service, session *model.BookingSession, offered []schedulingsvc.Slot, dateGiven bool) string {
	if session.ServiceID == nil {
		return "Which service would you like to book? We offer " + serviceNameList(services) + "."
	}

	code := serviceCodeByID(services, *session.ServiceID)
	if session.ProfessionalID == nil || session.SlotStartAt == nil {
		switch {
		case len(offered) > 0:
			return fmt.Sprintf("Here are some available times for Service %s — please pick one below.", code)
		case dateGiven:
			return fmt.Sprintf("I couldn't find any open times for Service %s on that date. Would you like to try another date?", code)
		default:
			return fmt.Sprintf("What date would you like to come in for Service %s?", code)
		}
	}

	if session.PatientName == nil {
		return "Great — what name should I put on the appointment?"
	}
	if session.PatientEmail == nil {
		return "Thanks! What email address should the confirmation be sent to?"
	}
	return "You're all set — please review and confirm your appointment below."
}

func serviceNameList(services []model.Service) string {
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, svc.DisplayName)
	}
	return strings.Join(names, ", ")
}

func serviceCodeByID(services []model.Service, id string) string {
	for _, svc := range services {
		if svc.ID == id {
			return svc.Code
		}
	}
	return ""
}

func codeKnown(knownCodes []string, code string) bool {
	return slices.Contains(knownCodes, code)
}

func validPatientName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	n := len([]rune(name))
	if n < 1 || n > 100 {
		return "", false
	}
	return name, true
}

func validPatientEmail(raw string) (string, bool) {
	email := strings.TrimSpace(raw)
	if len(email) < 1 || len(email) > 254 {
		return "", false
	}
	return email, true
}

func filterByTimeOfDay(slots []schedulingsvc.Slot, timeOfDay *string, loc *time.Location) []schedulingsvc.Slot {
	if timeOfDay == nil {
		return slots
	}
	filtered := make([]schedulingsvc.Slot, 0, len(slots))
	for _, slot := range slots {
		hour := slot.Start.In(loc).Hour()
		switch *timeOfDay {
		case "morning":
			if hour < 12 {
				filtered = append(filtered, slot)
			}
		case "afternoon":
			if hour >= 12 && hour < 17 {
				filtered = append(filtered, slot)
			}
		case "evening":
			if hour >= 17 {
				filtered = append(filtered, slot)
			}
		default:
			filtered = append(filtered, slot)
		}
	}
	return filtered
}
