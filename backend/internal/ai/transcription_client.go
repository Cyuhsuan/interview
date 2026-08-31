package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"backend/internal/service/transcription"
)

// transcriptionRequestTimeout is longer than chat completion's
// requestTimeout since it includes uploading the audio file, not just a
// short JSON payload.
const transcriptionRequestTimeout = 20 * time.Second

// contentTypeExtensions maps the whitelist enforced by
// internal/handler/voice to a filename extension, since the OpenAI-compatible
// audio transcription endpoint infers the audio format from the multipart
// file part's filename rather than trusting the Content-Type header alone.
var contentTypeExtensions = map[string]string{
	"audio/webm": "webm",
	"audio/ogg":  "ogg",
	"audio/mp4":  "mp4",
	"audio/wav":  "wav",
	"audio/mpeg": "mp3",
}

// TranscriptionClient is the sole concrete implementation of
// internal/service/transcription.Provider for this phase, mirroring
// Client's OpenAI-compatible HTTP integration style. Per
// backend/README.md's "Voice Transcription Endpoint", this is a
// development/test-time integration, not an approved production AI
// provider.
type TranscriptionClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewTranscriptionClient(baseURL, apiKey, model string) *TranscriptionClient {
	return &TranscriptionClient{
		httpClient: &http.Client{Timeout: transcriptionRequestTimeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

var _ transcription.Provider = (*TranscriptionClient)(nil)

type transcriptionResponse struct {
	Text string `json:"text"`
}

// Transcribe calls the configured OpenAI-compatible audio transcription
// endpoint. Any failure — network error, non-2xx status, or an unparseable
// response — is returned as an error; the caller
// (transcription.Service) converts it into the fail-closed
// ErrTranscriptionUnavailable.
func (c *TranscriptionClient) Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, transcriptionRequestTimeout)
	defer cancel()

	ext, ok := contentTypeExtensions[contentType]
	if !ok {
		ext = "bin"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	filePart, err := writer.CreateFormFile("file", "audio."+ext)
	if err != nil {
		return "", fmt.Errorf("build transcription request file part: %w", err)
	}
	if _, err := io.Copy(filePart, audio); err != nil {
		return "", fmt.Errorf("write transcription request audio: %w", err)
	}
	if err := writer.WriteField("model", c.model); err != nil {
		return "", fmt.Errorf("write transcription request model field: %w", err)
	}
	// Patients only interact in English (see root README's non-negotiable
	// product rules), so pinning the language reduces misrecognition into
	// another language.
	if err := writer.WriteField("language", "en"); err != nil {
		return "", fmt.Errorf("write transcription request language field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close transcription request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call AI transcription provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read AI transcription provider response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI transcription provider returned status %d", resp.StatusCode)
	}

	var parsed transcriptionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	if strings.TrimSpace(parsed.Text) == "" {
		return "", fmt.Errorf("AI transcription provider returned empty text")
	}

	return parsed.Text, nil
}
