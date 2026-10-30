// Package policy maps hosted delivery outcomes to an Envoy-safe action. It
// never substitutes raw context after the hosted service has declared that a
// sanitized fallback is needed.
package policy

import (
	"strings"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/nextmoca"
)

type Kind string

const (
	PassThrough Kind = "pass_through"
	Transform   Kind = "transform"
	Block       Kind = "block"
)

type Settings struct {
	Mode             string
	OnSelectionError string
}

type Receipt struct {
	Outcome         string
	Reason          string
	TokensBefore    int
	TokensAfter     int
	TokensSaved     int
	EngineLatencyMS float64
	SelectionID     string
}

type Decision struct {
	Kind    Kind
	Body    []byte
	Reason  string
	Receipt Receipt
}

type Rewriter interface {
	CandidateCount() int
	HasCandidate(string) bool
	RewriteSelected(map[string]string) ([]byte, error)
	RewriteSingleCandidate(string) ([]byte, error)
}

func Decide(settings Settings, original []byte, request Rewriter, response nextmoca.Response, selectErr error) Decision {
	receipt := Receipt{
		Outcome: response.Outcome, TokensBefore: response.TokensBefore, TokensAfter: response.TokensAfter,
		TokensSaved: response.TokensSaved, EngineLatencyMS: response.EngineLatencyMS, SelectionID: response.SelectionID,
	}
	mode := strings.ToLower(settings.Mode)
	if selectErr != nil {
		if isScreeningMode(mode) && strings.EqualFold(settings.OnSelectionError, "block") {
			return block("selection_error_blocked", receipt)
		}
		return pass(original, "selection_error", receipt)
	}
	if mode == "shadow" {
		return pass(original, "shadow", receipt)
	}

	switch strings.ToLower(response.Outcome) {
	case "selected", "engaged":
		return selected(original, request, response, receipt)
	case "sanitized_fallback":
		return sanitized(mode, original, request, response, receipt)
	case "blocked":
		if isScreeningMode(mode) {
			return block("blocked_by_nextmoca", receipt)
		}
		return pass(original, "blocked_observed_not_enforced", receipt)
	default:
		return pass(original, "unknown_delivery_outcome", receipt)
	}
}

func selected(original []byte, request Rewriter, response nextmoca.Response, receipt Receipt) Decision {
	if response.FallbackUsed || len(response.Selected) == 0 {
		return pass(original, "selection_not_deliverable", receipt)
	}
	excerpts := make(map[string]string, len(response.Selected))
	for _, selected := range response.Selected {
		if request.HasCandidate(selected.RecordID) && strings.TrimSpace(selected.Excerpt) != "" {
			excerpts[selected.RecordID] = selected.Excerpt
		}
	}
	if len(excerpts) == 0 {
		return pass(original, "selection_has_no_matching_candidate", receipt)
	}
	body, err := request.RewriteSelected(excerpts)
	if err != nil {
		return pass(original, "selection_rewrite_failed", receipt)
	}
	return transform(body, "selected", receipt)
}

func sanitized(mode string, original []byte, request Rewriter, response nextmoca.Response, receipt Receipt) Decision {
	if !isScreeningMode(mode) {
		return pass(original, "sanitized_fallback_observed_not_enabled", receipt)
	}
	if strings.TrimSpace(response.RenderedContext) == "" {
		return block("empty_sanitized_fallback", receipt)
	}
	if request.CandidateCount() != 1 {
		return block("sanitized_fallback_requires_record_mapping", receipt)
	}
	body, err := request.RewriteSingleCandidate(response.RenderedContext)
	if err != nil {
		return block("sanitized_fallback_rewrite_failed", receipt)
	}
	return transform(body, "sanitized_fallback", receipt)
}

func isScreeningMode(mode string) bool { return mode == "quarantine" || mode == "enforce" }

func pass(body []byte, reason string, receipt Receipt) Decision {
	receipt.Reason = reason
	return Decision{Kind: PassThrough, Body: body, Reason: reason, Receipt: receipt}
}

func transform(body []byte, reason string, receipt Receipt) Decision {
	receipt.Reason = reason
	return Decision{Kind: Transform, Body: body, Reason: reason, Receipt: receipt}
}

func block(reason string, receipt Receipt) Decision {
	receipt.Reason = reason
	return Decision{Kind: Block, Reason: reason, Receipt: receipt}
}
