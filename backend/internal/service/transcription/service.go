// Package transcription turns a recorded patient utterance into text via a
// provider-neutral port. Per backend/README.md's "Voice Transcription
// Endpoint", this service has no booking-relevant behavior of its own — the
// returned text is only ever fed back through the existing
// internal/service/conversation candidate-extraction path once the patient
// (or the frontend, per the auto-send UX) submits it as a chat message.
package transcription

import (
	"context"
	"errors"
	"io"
)

// Provider is the port a concrete AI transcription adapter (internal/ai)
// implements. It must only turn audio into text — it must never itself
// decide booking legality, matching the same boundary as
// internal/service/conversation.AIProvider.
type Provider interface {
	Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error)
}

// ErrTranscriptionUnavailable is returned whenever the underlying provider
// fails for any reason (timeout, non-2xx, unparseable response). Unlike
// conversation's AI extraction, there is no template text to fall back to
// here — the handler maps this to a fail-closed 503 so the frontend can
// prompt the patient to type instead.
var ErrTranscriptionUnavailable = errors.New("transcription unavailable")

type Service struct {
	provider Provider
}

func NewService(provider Provider) *Service {
	return &Service{provider: provider}
}

func (s *Service) Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error) {
	text, err := s.provider.Transcribe(ctx, audio, contentType)
	if err != nil {
		return "", ErrTranscriptionUnavailable
	}
	return text, nil
}
