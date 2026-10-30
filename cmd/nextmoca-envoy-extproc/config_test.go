package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/metrics"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/policy"
)

func TestLoadConfigAppliesSafeDefaults(t *testing.T) {
	t.Setenv("NEXTMOCA_API_KEY", "test-key")
	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if config.ListenAddress != ":9002" || config.HealthAddress != ":8080" || config.Runtime.Mode != "shadow" || config.Runtime.RequestAdapter != "auto" {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadConfigRequiresHostedAPIKey(t *testing.T) {
	t.Setenv("NEXTMOCA_API_KEY", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() error = nil, want missing API key error")
	}
}

func TestManagementHandlerServesHealthAndMetrics(t *testing.T) {
	telemetry := metrics.New()
	telemetry.ObserveDecision("active", policy.Decision{Kind: policy.PassThrough, Reason: "shadow"})
	handler := managementHandler(telemetry)

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK || health.Body.String() != `{"status":"ok"}` {
		t.Fatalf("health response = %d %q", health.Code, health.Body.String())
	}

	prometheus := httptest.NewRecorder()
	handler.ServeHTTP(prometheus, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if prometheus.Code != http.StatusOK || !strings.Contains(prometheus.Body.String(), "nextmoca_extproc_decisions_total") {
		t.Fatalf("metrics response = %d %q", prometheus.Code, prometheus.Body.String())
	}
}
