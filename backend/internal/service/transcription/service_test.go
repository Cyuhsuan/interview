package transcription

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeProvider struct {
	text string
	err  error
}

func (f *fakeProvider) Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error) {
	return f.text, f.err
}

func TestTranscribe_Success(t *testing.T) {
	svc := NewService(&fakeProvider{text: "I'd like a cleaning next Tuesday afternoon"})

	text, err := svc.Transcribe(context.Background(), strings.NewReader("audio bytes"), "audio/webm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "I'd like a cleaning next Tuesday afternoon" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestTranscribe_ProviderFailure(t *testing.T) {
	svc := NewService(&fakeProvider{err: errors.New("upstream timeout")})

	_, err := svc.Transcribe(context.Background(), strings.NewReader("audio bytes"), "audio/webm")
	if !errors.Is(err, ErrTranscriptionUnavailable) {
		t.Fatalf("expected ErrTranscriptionUnavailable, got: %v", err)
	}
}
