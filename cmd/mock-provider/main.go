// mock-provider records the exact request body and receipt headers that Envoy
// forwarded after the ExtProc bridge finished processing.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
)

func main() {
	listen := flag.String("listen", ":8082", "HTTP listen address")
	flag.Parse()
	http.HandleFunc("/", handle)
	log.Fatal(http.ListenAndServe(*listen, nil))
}

func handle(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, "read request", http.StatusBadRequest)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"received":         json.RawMessage(body),
		"nextmoca_headers": request.Header,
	})
}
