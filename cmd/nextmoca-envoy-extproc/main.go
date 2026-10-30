package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	envoyextproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/extproc"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/metrics"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/nextmoca"
	"google.golang.org/grpc"
)

type config struct {
	ListenAddress string
	HealthAddress string
	Runtime       extproc.Config
	Hosted        nextmoca.Config
}

func main() {
	if err := run(); err != nil {
		slog.Error("nextmoca envoy extproc stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", config.ListenAddress, err)
	}
	grpcServer := grpc.NewServer()
	telemetry := metrics.New()
	envoyextproc.RegisterExternalProcessorServer(grpcServer, extproc.NewServer(config.Runtime, nextmoca.NewHTTPClient(config.Hosted), telemetry))

	health := &http.Server{Addr: config.HealthAddress, Handler: managementHandler(telemetry)}
	go func() {
		if err := health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("health endpoint failed", "error", err)
		}
	}()
	go func() {
		slog.Info("Next Moca Envoy ExtProc listening", "address", config.ListenAddress, "mode", config.Runtime.Mode, "adapter", config.Runtime.RequestAdapter)
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("gRPC server failed", "error", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	grpcServer.GracefulStop()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return health.Shutdown(shutdownContext)
}

func managementHandler(telemetry *metrics.Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", telemetry.Handler())
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func loadConfig() (config, error) {
	apiKey := strings.TrimSpace(os.Getenv("NEXTMOCA_API_KEY"))
	if apiKey == "" {
		return config{}, errors.New("NEXTMOCA_API_KEY is required")
	}
	timeout, err := durationEnv("NEXTMOCA_TIMEOUT", 3*time.Second)
	if err != nil {
		return config{}, err
	}
	breakerCooldown, err := durationEnv("NEXTMOCA_BREAKER_COOLDOWN", 30*time.Second)
	if err != nil {
		return config{}, err
	}
	maxContextTokens, err := intEnv("NEXTMOCA_MAX_CONTEXT_TOKENS", 8192)
	if err != nil {
		return config{}, err
	}
	capacityTokens, err := intEnv("NEXTMOCA_CAPACITY_TOKENS", 128000)
	if err != nil {
		return config{}, err
	}
	breakerFailures, err := intEnv("NEXTMOCA_BREAKER_FAILURES", 3)
	if err != nil {
		return config{}, err
	}

	mode := env("NEXTMOCA_MODE", "shadow")
	if !oneOf(mode, "shadow", "active", "quarantine", "enforce") {
		return config{}, fmt.Errorf("NEXTMOCA_MODE %q is invalid", mode)
	}
	adapter := env("NEXTMOCA_REQUEST_ADAPTER", "auto")
	if !oneOf(adapter, "auto", "openai_chat", "openai_responses", "anthropic_messages", "gemini_generate_content") {
		return config{}, fmt.Errorf("NEXTMOCA_REQUEST_ADAPTER %q is invalid", adapter)
	}
	unsupported := env("NEXTMOCA_UNSUPPORTED_SHAPE", "pass")
	if !oneOf(unsupported, "pass", "block") {
		return config{}, fmt.Errorf("NEXTMOCA_UNSUPPORTED_SHAPE %q is invalid", unsupported)
	}
	onError := env("NEXTMOCA_ON_SELECTION_ERROR", "pass")
	if !oneOf(onError, "pass", "block") {
		return config{}, fmt.Errorf("NEXTMOCA_ON_SELECTION_ERROR %q is invalid", onError)
	}

	operatingPoint := env("NEXTMOCA_OPERATING_POINT", "np-2026-10-r8")
	return config{
		ListenAddress: env("NEXTMOCA_EXTPROC_LISTEN_ADDRESS", ":9002"),
		HealthAddress: env("NEXTMOCA_HEALTH_LISTEN_ADDRESS", ":8080"),
		Runtime:       extproc.Config{Mode: mode, RequestAdapter: adapter, UnsupportedShape: unsupported, OnSelectionError: onError, MaxContextTokens: maxContextTokens, CapacityTokens: capacityTokens, OperatingPoint: operatingPoint},
		Hosted:        nextmoca.Config{Endpoint: env("NEXTMOCA_ENDPOINT", "https://api.nextmoca.com"), APIKey: apiKey, Mode: mode, OperatingPoint: operatingPoint, MaxContextTokens: maxContextTokens, CapacityTokens: capacityTokens, Autochunk: true, Timeout: timeout, BreakerFailures: breakerFailures, BreakerCooldown: breakerCooldown},
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func intEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}
func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}
