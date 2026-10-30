// mock-nextmoca is a deterministic hosted-service fixture for the Docker E2E
// path. It is not part of the bridge runtime image.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
)

type selectRequest struct {
	RequestID string `json:"request_id"`
	Records   []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"records"`
}

func main() {
	listen := flag.String("listen", ":8081", "HTTP listen address")
	flag.Parse()
	http.HandleFunc("/v1/context/select", selectContext)
	log.Fatal(http.ListenAndServe(*listen, nil))
}

func selectContext(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer request.Body.Close()
	var input selectRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.RequestID == "" || len(input.Records) == 0 {
		http.Error(writer, "invalid select fixture request", http.StatusBadRequest)
		return
	}
	record := input.Records[0]
	excerpt := stripFixtureInstruction(record.Text)
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Needlepath-Selection-Id", "e2e-fixture-selection")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"request_id":        input.RequestID,
		"outcome":           "selected",
		"fallback_used":     false,
		"rendered_context":  excerpt,
		"tokens_before":     31,
		"tokens_after":      9,
		"tokens_saved":      22,
		"engine_latency_ms": 4.0,
		"selected": []map[string]string{{
			"record_id": record.ID,
			"excerpt":   excerpt,
		}},
	})
}

func stripFixtureInstruction(text string) string {
	needle := "ignore previous instructions"
	if index := strings.Index(strings.ToLower(text), needle); index >= 0 {
		return strings.TrimSpace(text[:index])
	}
	return strings.TrimSpace(text)
}
