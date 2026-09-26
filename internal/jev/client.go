package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Endpoint is the TypeSafe System One evaluation endpoint.
const Endpoint = "https://api.typesafe.ai/v1/systemone"

// Model is the model alias sent with every request.
const Model = "jev-latest"

// ErrMissingAPIKey reports a client without a usable API key. SystemOne
// returns it before building or sending a request.
var ErrMissingAPIKey = errors.New("jev: TYPESAFE_API_KEY is not set")

// Client talks to the TypeSafe System One API.
type Client struct {
	apiKey   string
	baseURL  string
	http     *http.Client
	backoff  time.Duration
	attempts int
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the internal HTTP client (7s timeout by default).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithBaseURL points the client at another host (tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// WithBackoff sets the base retry delay for 429/529 (default 250ms). A
// server-sent Retry-After hint can extend an individual delay (capped); see
// maxRetryAfter.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// WithAttempts sets how many total tries SystemOne makes (default 2).
// Values below 1 are clamped to 1.
func WithAttempts(n int) Option {
	return func(c *Client) {
		if n < 1 {
			n = 1
		}
		c.attempts = n
	}
}

// NewClient builds a client for the given API key. Surrounding whitespace is
// ignored; a blank key leaves the client unconfigured.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:   strings.TrimSpace(apiKey),
		baseURL:  Endpoint,
		http:     &http.Client{Timeout: 7 * time.Second},
		backoff:  250 * time.Millisecond,
		attempts: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Configured reports whether the client has a non-blank API key.
func (c *Client) Configured() bool {
	return c != nil && c.apiKey != ""
}

type request struct {
	State     json.RawMessage     `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// SystemOne evaluates the questions against state in a single call. All
// questions are answered in parallel server-side; the response carries one
// typed answer per question key.
func (c *Client) SystemOne(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	if !c.Configured() {
		return nil, ErrMissingAPIKey
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("jev: at least one question is required")
	}
	for key, q := range questions {
		if err := q.Validate(); err != nil {
			return nil, fmt.Errorf("jev: question %q: %w", key, err)
		}
	}

	// state is documented as string | object | array — nothing else.
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("jev: encoding state: %w", err)
	}
	if len(stateJSON) == 0 || !strings.ContainsRune(`"{[`, rune(stateJSON[0])) {
		return nil, fmt.Errorf("jev: state must be a string, object, or array")
	}

	payload, err := json.Marshal(request{State: stateJSON, Model: Model, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: encoding request: %w", err)
	}

	var lastErr error
	var retryAfter time.Duration
	attempts := max(c.attempts, 1)
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := c.backoff * time.Duration(1<<(attempt-1))
			if retryAfter > delay {
				delay = retryAfter
			}
			if err := sleepCtx(ctx, delay); err != nil {
				return nil, err
			}
		}
		var retryable bool
		var resp *Response
		resp, retryAfter, retryable, lastErr = c.do(ctx, payload)
		if lastErr == nil {
			return resp, nil
		}
		if !retryable {
			return nil, lastErr
		}
	}
	return nil, fmt.Errorf("jev: giving up after %d attempts: %w", attempts, lastErr)
}

// do performs one attempt. It reports the server's Retry-After hint (if any)
// alongside whether a failure is worth retrying (429 rate limit and 529
// overloaded per the API reference).
func (c *Client) do(ctx context.Context, payload []byte) (resp *Response, retryAfter time.Duration, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, false, fmt.Errorf("jev: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, true, fmt.Errorf("jev: request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 4<<20))
	if err != nil {
		return nil, 0, true, fmt.Errorf("jev: reading response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		retryable := httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode == 529
		var hint time.Duration
		if retryable {
			hint = cappedRetryAfter(httpResp.Header)
		}
		return nil, hint, retryable,
			&APIError{StatusCode: httpResp.StatusCode, Message: extractMessage(body), Raw: string(body)}
	}

	var decoded Response
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, 0, false, fmt.Errorf("jev: decoding response: %w", err)
	}
	if err := decoded.Validate(); err != nil {
		return nil, 0, false, err
	}
	return &decoded, 0, false, nil
}

// extractMessage pulls a human-readable message out of the common error
// envelopes; the API reference does not fix an error body schema. TypeSafe
// nests auth messages under "detail", so nested values are decoded loosely
// instead of assuming a string.
func extractMessage(body []byte) string {
	var envelope struct {
		Message string          `json:"message"`
		Detail  json.RawMessage `json:"detail"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return strings.TrimSpace(string(body))
	}
	if envelope.Message != "" {
		return envelope.Message
	}
	for _, raw := range []json.RawMessage{envelope.Detail, envelope.Error} {
		if msg := nestedMessage(raw); msg != "" {
			return msg
		}
	}
	return ""
}

// nestedMessage extracts a string message from a string, {"message":…}, or
// {"msg":…} error detail. Unknown shapes are ignored.
func nestedMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var obj struct {
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Message != "" {
			return obj.Message
		}
		return obj.Msg
	}
	return ""
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("jev: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// maxRetryAfter caps a server-sent Retry-After hint so one hostile or buggy
// header cannot stall a request indefinitely. Context cancellation still
// applies while waiting.
const maxRetryAfter = 30 * time.Second

// cappedRetryAfter parses the Retry-After hint, bounded by maxRetryAfter.
func cappedRetryAfter(header http.Header) time.Duration {
	if hint := RetryAfterHint(header); hint > 0 {
		return min(hint, maxRetryAfter)
	}
	return 0
}

// RetryAfterHint returns the seconds a 429/529 response asked for, or 0.
// Kept small and separate so tests can assert the parse without a round trip.
func RetryAfterHint(header http.Header) time.Duration {
	v := header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
