package main

import "testing"

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
