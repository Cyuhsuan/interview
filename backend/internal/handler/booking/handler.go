package booking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/model"
	"backend/internal/platform/httpproblem"
	bookingsvc "backend/internal/service/booking"
	catalogsvc "backend/internal/service/catalog"
)

// idempotencyKeyPattern matches README's Canonical Types Idempotency-Key
// format: 16-128 ASCII characters, letters/digits/./_/:/-only.
var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{16,128}$`)

type slotDTO struct {
	ProfessionalID string `json:"professionalId"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

type sessionResponse struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	ServiceCode  *string  `json:"serviceCode"`
	SelectedSlot *slotDTO `json:"selectedSlot"`
	PatientName  *string  `json:"patientName"`
	PatientEmail *string  `json:"patientEmail"`
	ExpiresAt    string   `json:"expiresAt"`
}

type appointmentResponse struct {
	ID             string `json:"id"`
	ServiceCode    string `json:"serviceCode"`
	ProfessionalID string `json:"professionalId"`
	PatientName    string `json:"patientName"`
	PatientEmail   string `json:"patientEmail"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

type slotRequest struct {
	ProfessionalID string `json:"professionalId"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

type patchSessionRequest struct {
	ServiceCode  *string      `json:"serviceCode"`
	SelectedSlot *slotRequest `json:"selectedSlot"`
	PatientName  *string      `json:"patientName"`
	PatientEmail *string      `json:"patientEmail"`
	Status       *string      `json:"status"`
}

type confirmAppointmentRequest struct {
	BookingSessionID string `json:"bookingSessionId"`
}

type Handler struct {
	service *bookingsvc.Service
	catalog *catalogsvc.Service
}

func NewHandler(service *bookingsvc.Service, catalog *catalogsvc.Service) *Handler {
	return &Handler{service: service, catalog: catalog}
}

func RegisterRoutes(engine *gin.Engine, h *Handler) {
	v1 := engine.Group("/api/v1")
	v1.POST("/booking-sessions", h.createSession)
	v1.GET("/booking-sessions/:id", h.getSession)
	v1.PATCH("/booking-sessions/:id", h.patchSession)
	v1.POST("/appointments", h.confirmAppointment)
}

func (h *Handler) createSession(c *gin.Context) {
	session, err := h.service.CreateSession(c.Request.Context())
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	resp, err := h.toSessionResponse(c.Request.Context(), session)
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	c.Header("ETag", etagFor(session.Version))
	c.Header("Location", "/api/v1/booking-sessions/"+session.ID)
	c.JSON(http.StatusCreated, resp)
}

func (h *Handler) getSession(c *gin.Context) {
	session, err := h.service.GetSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeSessionError(c, err)
		return
	}
	resp, err := h.toSessionResponse(c.Request.Context(), session)
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	c.Header("ETag", etagFor(session.Version))
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) patchSession(c *gin.Context) {
	version, ok := h.requireIfMatch(c)
	if !ok {
		return
	}

	var req patchSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "request body is not valid JSON", nil)
		return
	}

	patch := bookingsvc.SessionPatch{
		ServiceCode:     req.ServiceCode,
		PatientName:     req.PatientName,
		PatientEmail:    req.PatientEmail,
		RequestedStatus: req.Status,
	}
	if req.SelectedSlot != nil {
		start, err := time.Parse(time.RFC3339, req.SelectedSlot.Start)
		if err != nil {
			httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "selectedSlot.start is not a valid RFC 3339 timestamp", []httpproblem.FieldError{{Field: "selectedSlot.start", Code: "INVALID_FORMAT"}})
			return
		}
		end, err := time.Parse(time.RFC3339, req.SelectedSlot.End)
		if err != nil {
			httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "selectedSlot.end is not a valid RFC 3339 timestamp", []httpproblem.FieldError{{Field: "selectedSlot.end", Code: "INVALID_FORMAT"}})
			return
		}
		patch.SelectedSlot = &bookingsvc.SelectedSlot{
			ProfessionalID: req.SelectedSlot.ProfessionalID,
			Start:          start,
			End:            end,
			TimeZone:       req.SelectedSlot.TimeZone,
		}
	}

	session, err := h.service.UpdateSession(c.Request.Context(), c.Param("id"), version, patch)
	if err != nil {
		h.writeSessionError(c, err)
		return
	}
	resp, err := h.toSessionResponse(c.Request.Context(), session)
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	c.Header("ETag", etagFor(session.Version))
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) confirmAppointment(c *gin.Context) {
	version, ok := h.requireIfMatch(c)
	if !ok {
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "Idempotency-Key header is required", []httpproblem.FieldError{{Field: "Idempotency-Key", Code: "REQUIRED"}})
		return
	}
	if !idempotencyKeyPattern.MatchString(idempotencyKey) {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "Idempotency-Key does not match the required format", []httpproblem.FieldError{{Field: "Idempotency-Key", Code: "INVALID_FORMAT"}})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "request body could not be read", nil)
		return
	}
	var req confirmAppointmentRequest
	if err := json.Unmarshal(body, &req); err != nil || req.BookingSessionID == "" {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "bookingSessionId is required", []httpproblem.FieldError{{Field: "bookingSessionId", Code: "REQUIRED"}})
		return
	}

	hash := sha256.Sum256(body)
	requestHash := hex.EncodeToString(hash[:])

	appt, err := h.service.ConfirmAppointment(c.Request.Context(), req.BookingSessionID, version, idempotencyKey, requestHash)
	if err != nil {
		h.writeAppointmentError(c, err)
		return
	}

	serviceCode, err := h.serviceCodeForID(c.Request.Context(), &appt.ServiceID)
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	c.JSON(http.StatusCreated, appointmentResponse{
		ID:             appt.ID,
		ServiceCode:    derefOrEmpty(serviceCode),
		ProfessionalID: appt.ProfessionalID,
		PatientName:    appt.PatientName,
		PatientEmail:   appt.PatientEmail,
		Start:          appt.StartAt.Format(time.RFC3339),
		End:            appt.EndAt.Format(time.RFC3339),
		TimeZone:       appt.TimeZone,
	})
}

func (h *Handler) requireIfMatch(c *gin.Context) (int64, bool) {
	raw := c.GetHeader("If-Match")
	if raw == "" {
		httpproblem.Write(c, http.StatusPreconditionRequired, httpproblem.CodePreconditionRequired, "If-Match header is required", nil)
		return 0, false
	}
	trimmed := raw
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		trimmed = trimmed[1 : len(trimmed)-1]
	}
	version, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		httpproblem.Write(c, http.StatusPreconditionFailed, httpproblem.CodeSessionVersionMismatch, "If-Match is not a valid version", nil)
		return 0, false
	}
	return version, true
}

func (h *Handler) writeSessionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, bookingsvc.ErrSessionNotFound), errors.Is(err, bookingsvc.ErrSessionExpired):
		httpproblem.Write(c, http.StatusGone, httpproblem.CodeBookingSessionExpired, "This booking session no longer exists or has expired.", nil)
	case errors.Is(err, bookingsvc.ErrSessionVersionMismatch):
		httpproblem.Write(c, http.StatusPreconditionFailed, httpproblem.CodeSessionVersionMismatch, "The session has changed since If-Match was read.", nil)
	case errors.Is(err, bookingsvc.ErrInvalidStateTransition), errors.Is(err, bookingsvc.ErrValidationFailed):
		httpproblem.Write(c, http.StatusUnprocessableEntity, httpproblem.CodeValidationFailed, "The request is not valid for the session's current state.", nil)
	case errors.Is(err, bookingsvc.ErrSlotNoLongerAvailable):
		httpproblem.Write(c, http.StatusConflict, httpproblem.CodeSlotNoLongerAvailable, "The selected slot is no longer available.", nil)
	case errors.Is(err, catalogsvc.ErrInvalidServiceCode):
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "serviceCode does not match the required format", []httpproblem.FieldError{{Field: "serviceCode", Code: "INVALID_FORMAT"}})
	default:
		httpproblem.WriteInternal(c, err)
	}
}

func (h *Handler) writeAppointmentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, bookingsvc.ErrIdempotencyKeyReused):
		httpproblem.Write(c, http.StatusConflict, httpproblem.CodeIdempotencyKeyReused, "Idempotency-Key was reused with a different request.", nil)
	default:
		h.writeSessionError(c, err)
	}
}

func (h *Handler) toSessionResponse(ctx context.Context, session *model.BookingSession) (sessionResponse, error) {
	var serviceCode *string
	if session.ServiceID != nil {
		code, err := h.serviceCodeForID(ctx, session.ServiceID)
		if err != nil {
			return sessionResponse{}, err
		}
		serviceCode = code
	}

	var slot *slotDTO
	if session.ProfessionalID != nil && session.SlotStartAt != nil && session.SlotEndAt != nil {
		timeZone := ""
		if session.SlotTimeZone != nil {
			timeZone = *session.SlotTimeZone
		}
		slot = &slotDTO{
			ProfessionalID: *session.ProfessionalID,
			Start:          session.SlotStartAt.Format(time.RFC3339),
			End:            session.SlotEndAt.Format(time.RFC3339),
			TimeZone:       timeZone,
		}
	}

	return sessionResponse{
		ID:           session.ID,
		Status:       session.Status,
		ServiceCode:  serviceCode,
		SelectedSlot: slot,
		PatientName:  session.PatientName,
		PatientEmail: session.PatientEmail,
		ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func (h *Handler) serviceCodeForID(ctx context.Context, serviceID *string) (*string, error) {
	if serviceID == nil {
		return nil, nil
	}
	services, err := h.catalog.ListActiveServices(ctx)
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		if svc.ID == *serviceID {
			code := svc.Code
			return &code, nil
		}
	}
	return nil, nil
}

func etagFor(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
