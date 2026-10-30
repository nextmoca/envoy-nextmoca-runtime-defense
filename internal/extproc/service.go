// Package extproc implements Envoy's External Processing service as a thin
// protocol bridge to the hosted Next Moca Runtime Defense API.
package extproc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyextproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/adapter"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/metrics"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/nextmoca"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/policy"
)

type Config struct {
	Mode             string
	RequestAdapter   string
	UnsupportedShape string
	OnSelectionError string
	MaxContextTokens int
	CapacityTokens   int
	OperatingPoint   string
}

type Server struct {
	envoyextproc.UnimplementedExternalProcessorServer
	config   Config
	selector nextmoca.Selector
	metrics  *metrics.Metrics
}

func NewServer(config Config, selector nextmoca.Selector, reported ...*metrics.Metrics) *Server {
	if config.Mode == "" {
		config.Mode = "shadow"
	}
	if config.RequestAdapter == "" {
		config.RequestAdapter = "auto"
	}
	if config.UnsupportedShape == "" {
		config.UnsupportedShape = "pass"
	}
	if config.OnSelectionError == "" {
		config.OnSelectionError = "pass"
	}
	if config.MaxContextTokens == 0 {
		config.MaxContextTokens = 8192
	}
	if config.CapacityTokens == 0 {
		config.CapacityTokens = 128000
	}
	if config.OperatingPoint == "" {
		config.OperatingPoint = "np-2026-10-r8"
	}
	telemetry := metrics.New()
	if len(reported) > 0 && reported[0] != nil {
		telemetry = reported[0]
	}
	return &Server{config: config, selector: selector, metrics: telemetry}
}

func (s *Server) Process(stream envoyextproc.ExternalProcessor_ProcessServer) error {
	var requestContentType string
	var requestHeaders http.Header
	for {
		request, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch {
		case request.GetRequestHeaders() != nil:
			requestContentType = contentType(request.GetRequestHeaders().GetHeaders())
			requestHeaders = headerMapToHTTP(request.GetRequestHeaders().GetHeaders())
			if err := stream.Send(continueHeaders()); err != nil {
				return err
			}
		case request.GetRequestBody() != nil:
			body := request.GetRequestBody()
			response := s.processRequestBody(stream.Context(), body.GetBody(), body.GetEndOfStream(), requestContentType, requestHeaders)
			if err := stream.Send(response); err != nil {
				return err
			}
		case request.GetResponseHeaders() != nil:
			if err := stream.Send(&envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_ResponseHeaders{ResponseHeaders: &envoyextproc.HeadersResponse{Response: continueResponse()}}}); err != nil {
				return err
			}
		case request.GetResponseBody() != nil:
			if err := stream.Send(&envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_ResponseBody{ResponseBody: &envoyextproc.BodyResponse{Response: continueResponse()}}}); err != nil {
				return err
			}
		case request.GetRequestTrailers() != nil:
			if err := stream.Send(&envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_RequestTrailers{RequestTrailers: &envoyextproc.TrailersResponse{}}}); err != nil {
				return err
			}
		case request.GetResponseTrailers() != nil:
			if err := stream.Send(&envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_ResponseTrailers{ResponseTrailers: &envoyextproc.TrailersResponse{}}}); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported Envoy processing request")
		}
	}
}

func (s *Server) processRequestBody(ctx context.Context, body []byte, endOfStream bool, requestContentType string, requestHeaders http.Header) *envoyextproc.ProcessingResponse {
	if !endOfStream {
		return s.unsupported("unbuffered_request_body")
	}
	if !isJSON(requestContentType) {
		return s.unsupported("non_json_request")
	}
	request, err := adapter.Parse(body, requestHeaders, s.config.RequestAdapter)
	if err != nil {
		return s.unsupported("context_security_unsupported_request")
	}
	if request.CandidateCount() == 0 {
		s.metrics.ObserveDecision(s.config.Mode, policy.Decision{Kind: policy.PassThrough, Reason: "no_candidate_context"})
		return bodyResponse(nil, receiptHeaders(policy.Receipt{Reason: "no_candidate_context"}))
	}

	response, selectErr := s.selector.Select(ctx, nextmoca.Request{
		RequestID: newRequestID(), TaskPrompt: request.TaskPrompt, Records: request.Records,
	})
	decision := policy.Decide(policy.Settings{Mode: s.config.Mode, OnSelectionError: s.config.OnSelectionError}, body, request, response, selectErr)
	s.metrics.ObserveDecision(s.config.Mode, decision)
	if decision.Kind == policy.Block {
		return blockResponse(decision.Reason)
	}
	if decision.Kind == policy.Transform {
		return bodyResponse(decision.Body, receiptHeaders(decision.Receipt))
	}
	return bodyResponse(nil, receiptHeaders(decision.Receipt))
}

func (s *Server) unsupported(reason string) *envoyextproc.ProcessingResponse {
	if screeningMode(s.config.Mode) && strings.EqualFold(s.config.UnsupportedShape, "block") {
		s.metrics.ObserveDecision(s.config.Mode, policy.Decision{Kind: policy.Block, Reason: reason})
		return blockResponse(reason)
	}
	s.metrics.ObserveDecision(s.config.Mode, policy.Decision{Kind: policy.PassThrough, Reason: reason})
	return bodyResponse(nil, receiptHeaders(policy.Receipt{Reason: reason}))
}

func continueHeaders() *envoyextproc.ProcessingResponse {
	return &envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_RequestHeaders{
		RequestHeaders: &envoyextproc.HeadersResponse{Response: continueResponse()},
	}}
}

func bodyResponse(replacement []byte, headers *envoyextproc.HeaderMutation) *envoyextproc.ProcessingResponse {
	response := continueResponse()
	if replacement != nil {
		if headers == nil {
			headers = &envoyextproc.HeaderMutation{}
		}
		// Envoy rejects a body mutation when the forwarded Content-Length still
		// describes the pre-selection request body.
		headers.RemoveHeaders = append(headers.RemoveHeaders, "content-length")
		response.BodyMutation = &envoyextproc.BodyMutation{Mutation: &envoyextproc.BodyMutation_Body{Body: replacement}}
	}
	response.HeaderMutation = headers
	return &envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_RequestBody{
		RequestBody: &envoyextproc.BodyResponse{Response: response},
	}}
}

func continueResponse() *envoyextproc.CommonResponse {
	return &envoyextproc.CommonResponse{Status: envoyextproc.CommonResponse_CONTINUE}
}

func blockResponse(reason string) *envoyextproc.ProcessingResponse {
	status := typev3.StatusCode_Forbidden
	if reason == "selection_error_blocked" {
		status = typev3.StatusCode_ServiceUnavailable
	}
	if reason == "context_security_unsupported_request" || reason == "unbuffered_request_body" || reason == "non_json_request" {
		status = typev3.StatusCode_UnprocessableEntity
	}
	return &envoyextproc.ProcessingResponse{Response: &envoyextproc.ProcessingResponse_ImmediateResponse{
		ImmediateResponse: &envoyextproc.ImmediateResponse{
			Status: &typev3.HttpStatus{Code: status},
			Body:   []byte(`{"error":"nextmoca_context_governance_blocked"}`), Details: reason,
		},
	}}
}

func receiptHeaders(receipt policy.Receipt) *envoyextproc.HeaderMutation {
	values := map[string]string{
		"x-nextmoca-decision":      receipt.Reason,
		"x-nextmoca-outcome":       receipt.Outcome,
		"x-nextmoca-tokens-before": fmt.Sprintf("%d", receipt.TokensBefore),
		"x-nextmoca-tokens-after":  fmt.Sprintf("%d", receipt.TokensAfter),
		"x-nextmoca-tokens-saved":  fmt.Sprintf("%d", receipt.TokensSaved),
	}
	if receipt.SelectionID != "" {
		values["x-nextmoca-selection-id"] = receipt.SelectionID
	}
	setHeaders := make([]*corev3.HeaderValueOption, 0, len(values))
	for _, key := range []string{"x-nextmoca-decision", "x-nextmoca-outcome", "x-nextmoca-tokens-before", "x-nextmoca-tokens-after", "x-nextmoca-tokens-saved", "x-nextmoca-selection-id"} {
		if value, ok := values[key]; ok {
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{Header: &corev3.HeaderValue{Key: key, RawValue: []byte(value)}})
		}
	}
	return &envoyextproc.HeaderMutation{SetHeaders: setHeaders}
}

func contentType(headers *corev3.HeaderMap) string {
	if headers == nil {
		return ""
	}
	for _, header := range headers.Headers {
		if strings.EqualFold(header.Key, "content-type") {
			return string(header.RawValue)
		}
	}
	return ""
}

func headerMapToHTTP(headers *corev3.HeaderMap) http.Header {
	converted := make(http.Header)
	if headers == nil {
		return converted
	}
	for _, header := range headers.Headers {
		converted.Add(header.Key, string(header.RawValue))
	}
	return converted
}

func isJSON(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return contentType == "" || strings.Contains(contentType, "application/json") || strings.Contains(contentType, "+json")
}

func screeningMode(mode string) bool {
	return strings.EqualFold(mode, "quarantine") || strings.EqualFold(mode, "enforce")
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "extproc-fallback-request-id"
	}
	return "extproc-" + hex.EncodeToString(bytes)
}
