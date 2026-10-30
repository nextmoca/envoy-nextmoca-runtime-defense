package policy

import (
	"errors"
	"testing"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/adapter"
	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/nextmoca"
)

func TestQuarantineNeverRestoresRawContextAfterSanitizedFallback(t *testing.T) {
	request, err := adapter.Parse([]byte(`{"messages":[{"role":"tool","content":"unsafe raw context"},{"role":"user","content":"task"}]}`), nil, "openai_chat")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decision := Decide(Settings{Mode: "quarantine", OnSelectionError: "pass"}, []byte(`raw-body`), request, nextmoca.Response{
		Outcome: "sanitized_fallback", RenderedContext: "", FallbackUsed: true,
	}, nil)
	if decision.Kind != Block || decision.Reason != "empty_sanitized_fallback" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestSelectionFailureCanBeConfiguredToBlock(t *testing.T) {
	request, err := adapter.Parse([]byte(`{"messages":[{"role":"tool","content":"context"},{"role":"user","content":"task"}]}`), nil, "openai_chat")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decision := Decide(Settings{Mode: "enforce", OnSelectionError: "block"}, []byte(`raw-body`), request, nextmoca.Response{}, errors.New("timeout"))
	if decision.Kind != Block || decision.Reason != "selection_error_blocked" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestActiveObservesSanitizedFallbackWithoutDroppingRequest(t *testing.T) {
	request, err := adapter.Parse([]byte(`{"messages":[{"role":"tool","content":"context"},{"role":"user","content":"task"}]}`), nil, "openai_chat")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	original := []byte(`original-body`)
	decision := Decide(Settings{Mode: "active"}, original, request, nextmoca.Response{Outcome: "sanitized_fallback"}, nil)
	if decision.Kind != PassThrough || string(decision.Body) != string(original) {
		t.Fatalf("decision = %#v", decision)
	}
}
