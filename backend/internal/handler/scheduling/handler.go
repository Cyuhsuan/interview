package scheduling

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/platform/httpproblem"
	catalogsvc "backend/internal/service/catalog"
	schedulingsvc "backend/internal/service/scheduling"
)

type slotResponse struct {
	ProfessionalID string `json:"professionalId"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

type Handler struct {
	service  *schedulingsvc.Service
	timeZone string
}

func NewHandler(service *schedulingsvc.Service, timeZone string) *Handler {
	return &Handler{service: service, timeZone: timeZone}
}

func RegisterRoutes(engine *gin.Engine, h *Handler) {
	v1 := engine.Group("/api/v1")
	v1.GET("/availability", h.getAvailability)
}

func (h *Handler) getAvailability(c *gin.Context) {
	serviceCode := c.Query("serviceCode")
	if serviceCode == "" {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
			"serviceCode query parameter is required", []httpproblem.FieldError{
				{Field: "serviceCode", Code: "REQUIRED"},
			})
		return
	}

	date := c.Query("date")
	if date == "" {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
			"date query parameter is required", []httpproblem.FieldError{
				{Field: "date", Code: "REQUIRED"},
			})
		return
	}

	slots, err := h.service.GetAvailability(c.Request.Context(), serviceCode, date)
	if err != nil {
		switch {
		case errors.Is(err, catalogsvc.ErrInvalidServiceCode):
			httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
				"serviceCode does not match the required format", []httpproblem.FieldError{
					{Field: "serviceCode", Code: "INVALID_FORMAT"},
				})
		case errors.Is(err, schedulingsvc.ErrInvalidDate):
			httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
				"date does not match the required format", []httpproblem.FieldError{
					{Field: "date", Code: "INVALID_FORMAT"},
				})
		default:
			httpproblem.Write(c, http.StatusServiceUnavailable, httpproblem.CodeAvailabilityUnavailable,
				"Availability cannot be determined right now.", nil)
		}
		return
	}

	resp := make([]slotResponse, 0, len(slots))
	for _, slot := range slots {
		resp = append(resp, slotResponse{
			ProfessionalID: slot.ProfessionalID,
			Start:          slot.Start.Format(time.RFC3339),
			End:            slot.End.Format(time.RFC3339),
			TimeZone:       h.timeZone,
		})
	}
	c.JSON(http.StatusOK, resp)
}
