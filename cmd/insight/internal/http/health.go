package httphandler

import (
	"fmt"
	"net/http"
)

// healthCheck responds with 200 OK.
func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok"}`)
}
