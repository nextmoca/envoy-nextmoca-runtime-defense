package metrics

import (
	"testing"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/policy"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveDecisionTracksBoundedOperationalSignals(t *testing.T) {
	metrics := New()
	decision := policy.Decision{
		Kind:   policy.Transform,
		Reason: "selected",
		Receipt: policy.Receipt{
			Outcome:         "selected",
			TokensBefore:    120,
			TokensAfter:     40,
			TokensSaved:     80,
			EngineLatencyMS: 14.5,
		},
	}

	metrics.ObserveDecision("quarantine", decision)

	if got := testutil.ToFloat64(metrics.decisions.WithLabelValues("quarantine", "transform", "selected", "selected")); got != 1 {
		t.Fatalf("decision count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.tokens.WithLabelValues("saved")); got != 80 {
		t.Fatalf("saved tokens = %v, want 80", got)
	}
	if got := testutil.ToFloat64(metrics.engineLatency.WithLabelValues("selected")); got != 1 {
		t.Fatalf("engine latency observations = %v, want 1", got)
	}
}

func TestObserveDecisionBoundsUnknownResponseLabels(t *testing.T) {
	metrics := New()
	metrics.ObserveDecision("customer-controlled", policy.Decision{
		Kind:   policy.Block,
		Reason: "untrusted-reason-from-upstream",
		Receipt: policy.Receipt{
			Outcome: "untrusted-outcome-from-upstream",
		},
	})

	if got := testutil.ToFloat64(metrics.decisions.WithLabelValues("unknown", "block", "unknown", "other")); got != 1 {
		t.Fatalf("bounded decision count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.blocks.WithLabelValues("other")); got != 1 {
		t.Fatalf("block count = %v, want 1", got)
	}
}
