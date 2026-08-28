// Package ai is the sole concrete implementation of
// internal/service/conversation.AIProvider for this phase: an
// OpenAI-compatible Chat Completions HTTP client. Per backend/README.md's
// "AI Provider Adapter Contract", this is a development/test-time
// integration, not an approved production AI provider, and it must never
// decide booking legality — it only returns candidate values that
// internal/service/conversation re-validates deterministically.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend/internal/service/conversation"
)

// requestTimeout is a technical safety parameter, not a clinic-tunable
// setting, so it is a constant rather than an environment variable.
const requestTimeout = 8 * time.Second

var validOutOfScopeCategories = map[string]bool{
	"":                  true,
	"diagnosis":         true,
	"prescription":      true,
	"emergency":         true,
	"price":             true,
	"insurance":         true,
	"cancel_reschedule": true,
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

var _ conversation.AIProvider = (*Client)(nil)

type extractionJSON struct {
	OutOfScopeCategory string  `json:"outOfScopeCategory"`
	ServiceCode        *string `json:"serviceCode"`
	DateISO            *string `json:"dateISO"`
	TimeOfDay          *string `json:"timeOfDay"`
	PatientName        *string `json:"patientName"`
	PatientEmail       *string `json:"patientEmail"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Extract calls the configured OpenAI-compatible endpoint and parses its
// JSON output into a conversation.Extraction. Any failure — network error,
// non-2xx status, or a response that doesn't parse as the expected JSON
// schema — is returned as an error; the caller (conversation.Service) is
// responsible for falling back to a safe clarifying reply instead of
// applying anything.
func (c *Client) Extract(ctx context.Context, message string, ref time.Time, knownServiceCodes []string) (conversation.Extraction, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt(ref, knownServiceCodes)},
			{Role: "user", Content: message},
		},
		Temperature:    0,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return conversation.Extraction{}, fmt.Errorf("encode chat completion request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return conversation.Extraction{}, fmt.Errorf("build chat completion request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return conversation.Extraction{}, fmt.Errorf("call AI provider: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return conversation.Extraction{}, fmt.Errorf("read AI provider response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return conversation.Extraction{}, fmt.Errorf("AI provider returned status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return conversation.Extraction{}, fmt.Errorf("decode chat completion response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return conversation.Extraction{}, fmt.Errorf("AI provider returned no choices")
	}

	var extracted extractionJSON
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &extracted); err != nil {
		return conversation.Extraction{}, fmt.Errorf("decode extraction JSON: %w", err)
	}
	if !validOutOfScopeCategories[extracted.OutOfScopeCategory] {
		extracted.OutOfScopeCategory = ""
	}

	return conversation.Extraction{
		OutOfScopeCategory: extracted.OutOfScopeCategory,
		ServiceCode:        emptyToNil(extracted.ServiceCode),
		DateISO:            emptyToNil(extracted.DateISO),
		TimeOfDay:          emptyToNil(extracted.TimeOfDay),
		PatientName:        emptyToNil(extracted.PatientName),
		PatientEmail:       emptyToNil(extracted.PatientEmail),
	}, nil
}

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func systemPrompt(ref time.Time, knownServiceCodes []string) string {
	return fmt.Sprintf(`You are a candidate-value extractor for a dental clinic booking chat. You do not make booking decisions and you never diagnose, prescribe, quote prices, or discuss insurance — you only extract structured candidate values from the patient's message.

Today's date/time (for resolving relative dates like "next Tuesday") is %s.
Known service codes: %s.

Reply with ONLY a JSON object matching this exact shape, using null for anything not present in the message:
{
  "outOfScopeCategory": one of "" | "diagnosis" | "prescription" | "emergency" | "price" | "insurance" | "cancel_reschedule",
  "serviceCode": string or null (must be one of the known service codes, or null),
  "dateISO": string or null (YYYY-MM-DD),
  "timeOfDay": one of "morning" | "afternoon" | "evening" or null,
  "patientName": string or null,
  "patientEmail": string or null
}

Set outOfScopeCategory whenever the message asks for diagnosis, medication/prescription advice, describes a medical emergency, asks for pricing/quotes, asks about insurance, or asks to cancel/reschedule an appointment. When outOfScopeCategory is set, still fill in any other fields you can read from the message.`,
		ref.Format(time.RFC3339), strings.Join(knownServiceCodes, ", "))
}
