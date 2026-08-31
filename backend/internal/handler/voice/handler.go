// Package voice implements the HTTP transformation layer for
// POST /voice/transcriptions, per backend/README.md's "Voice Transcription
// Endpoint" contract. It only converts requests/responses — the actual
// transcription call lives in internal/service/transcription.
package voice

import (
	"errors"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/platform/httpproblem"
	transcriptionsvc "backend/internal/service/transcription"
)

// maxAudioBytes is an exception to the global 64 KiB JSON body cap
// documented in backend/README.md's "RESTful API Contract" — audio
// recordings need a much larger, explicit limit of their own.
const maxAudioBytes = 10 << 20 // 10 MiB

var allowedContentTypes = map[string]bool{
	"audio/webm": true,
	"audio/ogg":  true,
	"audio/mp4":  true,
	"audio/wav":  true,
	"audio/mpeg": true,
}

type transcriptionResponse struct {
	Text string `json:"text"`
}

type Handler struct {
	service *transcriptionsvc.Service
}

func NewHandler(service *transcriptionsvc.Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(engine *gin.Engine, h *Handler) {
	v1 := engine.Group("/api/v1")
	v1.POST("/voice/transcriptions", h.transcribe)
}

func (h *Handler) transcribe(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAudioBytes)

	fileHeader, err := c.FormFile("audio")
	if err != nil {
		h.writeUploadError(c, err, "audio file is required", "REQUIRED")
		return
	}

	// Real recorders (e.g. the browser's MediaRecorder) report a MIME type
	// with codec parameters, such as "audio/webm;codecs=opus" — parse out
	// just the base type before checking it against the whitelist.
	baseContentType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil || !allowedContentTypes[baseContentType] {
		httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, "audio content type is not supported", []httpproblem.FieldError{{Field: "audio", Code: "INVALID_FORMAT"}})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		h.writeUploadError(c, err, "audio file could not be read", "INVALID")
		return
	}
	defer file.Close()

	text, err := h.service.Transcribe(c.Request.Context(), file, baseContentType)
	if err != nil {
		if errors.Is(err, transcriptionsvc.ErrTranscriptionUnavailable) {
			httpproblem.Write(c, http.StatusServiceUnavailable, httpproblem.CodeVoiceTranscriptionUnavailable, "Voice transcription is not available right now. Please type your message instead.", nil)
			return
		}
		httpproblem.WriteInternal(c, err)
		return
	}

	c.JSON(http.StatusOK, transcriptionResponse{Text: text})
}

func (h *Handler) writeUploadError(c *gin.Context, err error, detail, fieldCode string) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		httpproblem.Write(c, http.StatusRequestEntityTooLarge, httpproblem.CodeRequestTooLarge, "audio exceeds the maximum upload size", nil)
		return
	}
	httpproblem.Write(c, http.StatusBadRequest, httpproblem.CodeInvalidRequest, detail, []httpproblem.FieldError{{Field: "audio", Code: fieldCode}})
}
