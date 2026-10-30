// Package nextmoca is the thin, fail-configurable client for the hosted
// Next Moca Runtime Defense API. It deliberately contains no selection or
// security policy logic.
package nextmoca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	selectPath       = "/v1/context/select"
	maxWireBytes     = 6 * 1024 * 1024
	defaultTimeout   = 3 * time.Second
	defaultFailures  = 3
	defaultCooldown  = 30 * time.Second
)

type Config struct {
	Endpoint         string
	APIKey           string
	Mode             string
	OperatingPoint   string
	MaxContextTokens int
	CapacityTokens   int
	Autochunk        bool
	Timeout          time.Duration
	BreakerFailures  int
	BreakerCooldown  time.Duration
}

type Record struct {
	ID         string
	Kind       string
	Source     string
	SourceKind string
	Text       string
}

type Request struct {
	RequestID  string
	TaskPrompt string
	Records    []Record
}

type SelectedRecord struct {
	RecordID string `json:"record_id"`
	Excerpt  string `json:"excerpt"`
}

type Response struct {
	RequestID       string
	SelectionID     string
	Outcome         string
	RenderedContext string
	TokensBefore    int
	TokensAfter     int
	TokensSaved     int
	EngineLatencyMS float64
	FallbackUsed    bool
	Selected        []SelectedRecord
}

type Selector interface {
	Select(context.Context, Request) (Response, error)
}

type Client struct {
	config  Config
	http    *http.Client
	breaker *breaker
}

func NewHTTPClient(config Config) *Client {
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.BreakerFailures <= 0 {
		config.BreakerFailures = defaultFailures
	}
	if config.BreakerCooldown <= 0 {
		config.BreakerCooldown = defaultCooldown
	}
	return &Client{
		config: config,
		http: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		breaker: newBreaker(config.BreakerFailures, config.BreakerCooldown),
	}
}

func (c *Client) Select(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.RequestID) == "" {
		return Response{}, errors.New("request_id is required")
	}
	if strings.TrimSpace(request.TaskPrompt) == "" {
		return Response{}, errors.New("task prompt is required")
	}
	if !c.breaker.Allow() {
		return Response{}, errors.New("hosted Next Moca circuit breaker is open")
	}

	body, err := json.Marshal(c.wireRequest(request))
	if err != nil {
		return Response{}, fmt.Errorf("encode select request: %w", err)
	}
	if len(body) > maxWireBytes {
		return Response{}, fmt.Errorf("select request exceeds %d bytes", maxWireBytes)
	}

	callContext, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(callContext, http.MethodPost, strings.TrimRight(c.config.Endpoint, "/")+selectPath, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create select request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		c.breaker.Report(true)
		return Response{}, fmt.Errorf("call hosted Next Moca: %w", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		c.breaker.Report(httpResponse.StatusCode >= http.StatusInternalServerError)
		return Response{}, fmt.Errorf("hosted Next Moca returned HTTP %d", httpResponse.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxWireBytes+1))
	if err != nil {
		c.breaker.Report(true)
		return Response{}, fmt.Errorf("read select response: %w", err)
	}
	if len(raw) > maxWireBytes {
		c.breaker.Report(false)
		return Response{}, fmt.Errorf("select response exceeds %d bytes", maxWireBytes)
	}
	response, err := decodeResponse(raw, request.RequestID)
	if err != nil {
		c.breaker.Report(false)
		return Response{}, err
	}
	response.SelectionID = httpResponse.Header.Get("X-Needlepath-Selection-Id")
	c.breaker.Report(false)
	return response, nil
}

func (c *Client) wireRequest(request Request) map[string]any {
	records := make([]map[string]string, 0, len(request.Records))
	for _, record := range request.Records {
		records = append(records, map[string]string{
			"id": record.ID, "kind": record.Kind, "source": record.Source,
			"source_kind": record.SourceKind, "text": record.Text,
		})
	}
	return map[string]any{
		"request_id": request.RequestID,
		"records": records,
		"task": map[string]string{"prompt": request.TaskPrompt},
		"budget": map[string]any{
			"max_context_tokens": c.config.MaxContextTokens,
			"capacity_cap_tokens": c.config.CapacityTokens,
			"operating_point": c.config.OperatingPoint,
			"autochunk": c.config.Autochunk,
		},
		"render": true, "render_format": "plain", "return_per_record": true,
		"security_mode": securityMode(c.config.Mode),
	}
}

func securityMode(mode string) string {
	switch strings.ToLower(mode) {
	case "shadow":
		return "audit"
	case "quarantine":
		return "quarantine"
	case "enforce":
		return "enforce"
	default:
		return "off"
	}
}

func decodeResponse(raw []byte, expectedRequestID string) (Response, error) {
	var wire struct {
		RequestID       string `json:"request_id"`
		Outcome         string `json:"outcome"`
		FallbackUsed    *bool  `json:"fallback_used"`
		RenderedContext *string `json:"rendered_context"`
		TokensBefore    *int `json:"tokens_before"`
		TokensAfter     *int `json:"tokens_after"`
		TokensSaved     *int `json:"tokens_saved"`
		EngineLatencyMS *float64 `json:"engine_latency_ms"`
		Selected        []SelectedRecord `json:"selected"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Response{}, fmt.Errorf("decode select response: %w", err)
	}
	if wire.RequestID == "" || wire.RequestID != expectedRequestID {
		return Response{}, errors.New("invalid select response: request_id does not match")
	}
	if wire.Outcome == "" || wire.FallbackUsed == nil || wire.RenderedContext == nil || wire.TokensBefore == nil || wire.TokensAfter == nil || wire.TokensSaved == nil || wire.EngineLatencyMS == nil {
		return Response{}, errors.New("invalid select response: required delivery fields are missing")
	}
	for _, selected := range wire.Selected {
		if selected.RecordID == "" {
			return Response{}, errors.New("invalid select response: selected record_id is required")
		}
	}
	return Response{
		RequestID: wire.RequestID, Outcome: wire.Outcome, FallbackUsed: *wire.FallbackUsed,
		RenderedContext: *wire.RenderedContext, TokensBefore: *wire.TokensBefore,
		TokensAfter: *wire.TokensAfter, TokensSaved: *wire.TokensSaved,
		EngineLatencyMS: *wire.EngineLatencyMS, Selected: wire.Selected,
	}, nil
}

type breaker struct {
	mu           sync.Mutex
	failures     int
	limit        int
	cooldown     time.Duration
	openedUntil  time.Time
}

func newBreaker(limit int, cooldown time.Duration) *breaker {
	return &breaker{limit: limit, cooldown: cooldown}
}

func (b *breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.openedUntil.IsZero() || !time.Now().Before(b.openedUntil)
}

func (b *breaker) Report(transientFailure bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !transientFailure {
		b.failures = 0
		b.openedUntil = time.Time{}
		return
	}
	b.failures++
	if b.failures >= b.limit {
		b.openedUntil = time.Now().Add(b.cooldown)
	}
}
