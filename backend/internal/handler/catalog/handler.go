package catalog

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/platform/httpproblem"
	catalogsvc "backend/internal/service/catalog"
)

type serviceResponse struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	DisplayName     string `json:"displayName"`
	DurationMinutes int16  `json:"durationMinutes"`
}

type professionalResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	DisplayName string `json:"displayName"`
}

type Handler struct {
	service *catalogsvc.Service
}

func NewHandler(service *catalogsvc.Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes mounts the Catalog module's routes under /api/v1.
func RegisterRoutes(engine *gin.Engine, h *Handler) {
	v1 := engine.Group("/api/v1")
	v1.GET("/services", h.listServices)
	v1.GET("/professionals", h.listProfessionals)
}

func (h *Handler) listServices(c *gin.Context) {
	services, err := h.service.ListActiveServices(c.Request.Context())
	if err != nil {
		httpproblem.WriteInternal(c, err)
		return
	}

	resp := make([]serviceResponse, 0, len(services))
	for _, s := range services {
		resp = append(resp, serviceResponse{
			ID:              s.ID,
			Code:            s.Code,
			DisplayName:     s.DisplayName,
			DurationMinutes: s.DurationMinutes,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) listProfessionals(c *gin.Context) {
	serviceCode := c.Query("serviceCode")
	if serviceCode == "" {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
			"serviceCode query parameter is required", []httpproblem.FieldError{
				{Field: "serviceCode", Code: "REQUIRED"},
			})
		return
	}

	professionals, err := h.service.ListQualifiedProfessionals(c.Request.Context(), serviceCode)
	if err != nil {
		if errors.Is(err, catalogsvc.ErrInvalidServiceCode) {
			httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest,
				"serviceCode does not match the required format", []httpproblem.FieldError{
					{Field: "serviceCode", Code: "INVALID_FORMAT"},
				})
			return
		}
		httpproblem.WriteInternal(c, err)
		return
	}

	resp := make([]professionalResponse, 0, len(professionals))
	for _, p := range professionals {
		resp = append(resp, professionalResponse{ID: p.ID, Code: p.Code, DisplayName: p.DisplayName})
	}
	c.JSON(http.StatusOK, resp)
}
