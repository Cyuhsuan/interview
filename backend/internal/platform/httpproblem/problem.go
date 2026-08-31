// Package httpproblem writes application/problem+json error responses per
// backend/README.md's Error Contract. It is a platform-level HTTP concern
// usable by any handler, not owned by a specific feature module.
package httpproblem

import "github.com/gin-gonic/gin"

const ContentType = "application/problem+json"

// Error codes from backend/README.md's Error Contract table.
const (
	CodeInvalidRequest                = "INVALID_REQUEST"
	CodeInternalError                 = "INTERNAL_ERROR"
	CodeSlotNoLongerAvailable         = "SLOT_NO_LONGER_AVAILABLE"
	CodeIdempotencyKeyReused          = "IDEMPOTENCY_KEY_REUSED"
	CodeBookingSessionExpired         = "BOOKING_SESSION_EXPIRED"
	CodeSessionVersionMismatch        = "SESSION_VERSION_MISMATCH"
	CodeRequestTooLarge               = "REQUEST_TOO_LARGE"
	CodeValidationFailed              = "VALIDATION_FAILED"
	CodePreconditionRequired          = "PRECONDITION_REQUIRED"
	CodeAvailabilityUnavailable       = "AVAILABILITY_UNAVAILABLE"
	CodeVoiceTranscriptionUnavailable = "VOICE_TRANSCRIPTION_UNAVAILABLE"
	// RATE_LIMITED is not yet produced by any handler — no rate-limit
	// middleware exists yet — so its constant is intentionally omitted
	// until that middleware is added.
)

var titles = map[string]string{
	CodeInvalidRequest:                "Invalid Request",
	CodeInternalError:                 "Internal Error",
	CodeSlotNoLongerAvailable:         "Slot No Longer Available",
	CodeIdempotencyKeyReused:          "Idempotency Key Reused",
	CodeBookingSessionExpired:         "Booking Session Expired",
	CodeSessionVersionMismatch:        "Session Version Mismatch",
	CodeRequestTooLarge:               "Request Too Large",
	CodeValidationFailed:              "Validation Failed",
	CodePreconditionRequired:          "Precondition Required",
	CodeAvailabilityUnavailable:       "Availability Unavailable",
	CodeVoiceTranscriptionUnavailable: "Voice Transcription Unavailable",
}

type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

type Problem struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Code     string       `json:"code"`
	Detail   string       `json:"detail"`
	Instance string       `json:"instance"`
	Errors   []FieldError `json:"errors,omitempty"`
}

// Write sends a problem+json response using the request path as the
// request-scoped `instance` required by the Error Contract.
func Write(c *gin.Context, status int, code, detail string, fieldErrors []FieldError) {
	c.Header("Content-Type", ContentType)
	c.AbortWithStatusJSON(status, Problem{
		Type:     "about:blank",
		Title:    titles[code],
		Status:   status,
		Code:     code,
		Detail:   detail,
		Instance: c.Request.URL.Path,
		Errors:   fieldErrors,
	})
}

// WriteInternal records err server-side and writes a safe, detail-free 500
// body — it must never leak err.Error() into the response, per the Error
// Contract's ban on stack traces/SQL/patient data in `detail`.
func WriteInternal(c *gin.Context, err error) {
	_ = c.Error(err)
	Write(c, 500, CodeInternalError, "An unexpected error occurred.", nil)
}
