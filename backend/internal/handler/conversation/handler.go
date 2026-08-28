// Package conversation implements the HTTP transformation layer for
// POST /booking-sessions/{id}/messages, per backend/README.md's contract.
// It only converts requests/responses — all conversation logic lives in
// internal/service/conversation.
package conversation

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/platform/httpproblem"
	bookingsvc "backend/internal/service/booking"
	catalogsvc "backend/internal/service/catalog"
	conversationsvc "backend/internal/service/conversation"
)

const maxMessageCodePoints = 2000

type slotDTO struct {
	ProfessionalID string `json:"professionalId"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

type messageRequest struct {
	Message string `json:"message"`
}

type messageResponse struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	ServiceCode  *string   `json:"serviceCode"`
	SelectedSlot *slotDTO  `json:"selectedSlot"`
	PatientName  *string   `json:"patientName"`
	PatientEmail *string   `json:"patientEmail"`
	ExpiresAt    string    `json:"expiresAt"`
	Reply        string    `json:"reply"`
	OfferedSlots []slotDTO `json:"offeredSlots"`
	OutOfScope   bool      `json:"outOfScope"`
}

type Handler struct {
	service  *conversationsvc.Service
	catalog  *catalogsvc.Service
	timeZone string
}

func NewHandler(service *conversationsvc.Service, catalog *catalogsvc.Service, timeZone string) *Handler {
	return &Handler{service: service, catalog: catalog, timeZone: timeZone}
}

func RegisterRoutes(engine *gin.Engine, h *Handler) {
	v1 := engine.Group("/api/v1")
	v1.POST("/booking-sessions/:id/messages", h.sendMessage)
}

func (h *Handler) sendMessage(c *gin.Context) {
	var req messageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "request body is not valid JSON", nil)
		return
	}

	message := strings.TrimSpace(req.Message)
	n := len([]rune(message))
	if n < 1 {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "message is required", []httpproblem.FieldError{{Field: "message", Code: "REQUIRED"}})
		return
	}
	if n > maxMessageCodePoints {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "message exceeds the maximum length", []httpproblem.FieldError{{Field: "message", Code: "TOO_LONG"}})
		return
	}

	reply, err := h.service.SendMessage(c.Request.Context(), c.Param("id"), message)
	if err != nil {
		h.writeError(c, err)
		return
	}

	resp, err := h.toResponse(c.Request.Context(), reply)
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}
	c.Header("ETag", `"`+strconv.FormatInt(reply.Session.Version, 10)+`"`)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, bookingsvc.ErrSessionNotFound), errors.Is(err, bookingsvc.ErrSessionExpired):
		httpproblem.Write(c, http.StatusGone, httpproblem.CodeBookingSessionExpired, "This booking session no longer exists or has expired.", nil)
	default:
		// SendMessage only returns non-session errors when it fails to
		// read Catalog/Scheduling data from PostgreSQL while computing
		// offeredSlots — fail-closed per README's availability rule.
		httpproblem.Write(c, http.StatusServiceUnavailable, httpproblem.CodeAvailabilityUnavailable, "Availability cannot be determined right now.", nil)
	}
}

func (h *Handler) toResponse(ctx context.Context, reply conversationsvc.Reply) (messageResponse, error) {
	session := reply.Session

	var serviceCode *string
	if session.ServiceID != nil {
		code, err := h.serviceCodeForID(ctx, *session.ServiceID)
		if err != nil {
			return messageResponse{}, err
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

	offered := make([]slotDTO, 0, len(reply.OfferedSlots))
	for _, s := range reply.OfferedSlots {
		offered = append(offered, slotDTO{
			ProfessionalID: s.ProfessionalID,
			Start:          s.Start.Format(time.RFC3339),
			End:            s.End.Format(time.RFC3339),
			TimeZone:       h.timeZone,
		})
	}

	return messageResponse{
		ID:           session.ID,
		Status:       session.Status,
		ServiceCode:  serviceCode,
		SelectedSlot: slot,
		PatientName:  session.PatientName,
		PatientEmail: session.PatientEmail,
		ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
		Reply:        reply.Text,
		OfferedSlots: offered,
		OutOfScope:   reply.OutOfScope,
	}, nil
}

func (h *Handler) serviceCodeForID(ctx context.Context, serviceID string) (*string, error) {
	services, err := h.catalog.ListActiveServices(ctx)
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		if svc.ID == serviceID {
			code := svc.Code
			return &code, nil
		}
	}
	return nil, nil
}
