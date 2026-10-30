// Package metrics exposes bounded, Prometheus-compatible operational signals
// for the thin Envoy bridge. It never emits request content or identifiers.
package metrics

import (
	"net/http"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/policy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry             *prometheus.Registry
	decisions            *prometheus.CounterVec
	blocks               *prometheus.CounterVec
	tokens               *prometheus.CounterVec
	engineLatency        *prometheus.CounterVec
	engineLatencySeconds *prometheus.HistogramVec
}

func New() *Metrics {
	metrics := &Metrics{registry: prometheus.NewRegistry()}
	metrics.decisions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nextmoca_extproc_decisions_total",
		Help: "Number of bridge delivery decisions.",
	}, []string{"mode", "decision", "outcome", "reason"})
	metrics.blocks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nextmoca_extproc_blocks_total",
		Help: "Number of requests blocked by the bridge.",
	}, []string{"reason"})
	metrics.tokens = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nextmoca_extproc_tokens_total",
		Help: "Hosted Next Moca token accounting reported by the bridge.",
	}, []string{"direction"})
	metrics.engineLatency = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nextmoca_extproc_engine_latency_observations_total",
		Help: "Number of hosted engine latency observations.",
	}, []string{"outcome"})
	metrics.engineLatencySeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nextmoca_extproc_engine_latency_seconds",
		Help:    "Hosted Next Moca engine latency reported by the bridge.",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})
	metrics.registry.MustRegister(metrics.decisions, metrics.blocks, metrics.tokens, metrics.engineLatency, metrics.engineLatencySeconds)
	return metrics
}

func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return promhttp.Handler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveDecision(mode string, decision policy.Decision) {
	if m == nil {
		return
	}
	outcome := boundedOutcome(decision.Receipt.Outcome)
	reason := boundedReason(decision.Reason)
	m.decisions.WithLabelValues(boundedMode(mode), boundedDecision(string(decision.Kind)), outcome, reason).Inc()
	m.tokens.WithLabelValues("before").Add(nonNegative(decision.Receipt.TokensBefore))
	m.tokens.WithLabelValues("after").Add(nonNegative(decision.Receipt.TokensAfter))
	m.tokens.WithLabelValues("saved").Add(nonNegative(decision.Receipt.TokensSaved))
	if decision.Receipt.EngineLatencyMS > 0 {
		m.engineLatency.WithLabelValues(outcome).Inc()
		m.engineLatencySeconds.WithLabelValues(outcome).Observe(decision.Receipt.EngineLatencyMS / 1000)
	}
	if decision.Kind == policy.Block {
		m.ObserveBlock(decision.Reason)
	}
}

func (m *Metrics) ObserveBlock(reason string) {
	if m == nil {
		return
	}
	m.blocks.WithLabelValues(boundedReason(reason)).Inc()
}

func nonNegative(value int) float64 {
	if value < 0 {
		return 0
	}
	return float64(value)
}

func boundedMode(value string) string {
	switch value {
	case "shadow", "active", "quarantine", "enforce":
		return value
	default:
		return "unknown"
	}
}

func boundedDecision(value string) string {
	switch value {
	case string(policy.PassThrough), string(policy.Transform), string(policy.Block):
		return value
	default:
		return "unknown"
	}
}

func boundedOutcome(value string) string {
	switch value {
	case "selected", "engaged", "sanitized_fallback", "blocked":
		return value
	default:
		return "unknown"
	}
}

func boundedReason(value string) string {
	switch value {
	case "selected", "sanitized_fallback", "shadow", "selection_error", "selection_error_blocked", "selection_not_deliverable", "selection_has_no_matching_candidate", "selection_rewrite_failed", "empty_sanitized_fallback", "sanitized_fallback_requires_record_mapping", "sanitized_fallback_rewrite_failed", "blocked_by_nextmoca", "blocked_observed_not_enforced", "unknown_delivery_outcome", "context_security_unsupported_request", "unbuffered_request_body", "non_json_request", "no_candidate_context":
		return value
	default:
		return "other"
	}
}
