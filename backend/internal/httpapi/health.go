package httpapi

import (
	"context"
	"net/http"
	"time"
)

// Ping is the only database behavior needed by health checks.
type Ping func(context.Context) error

func Handler(ping Ping, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, `{"status":"ok"}`) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := ping(ctx); err != nil {
			respond(w, http.StatusServiceUnavailable, `{"status":"unavailable"}`)
			return
		}
		respond(w, http.StatusOK, `{"status":"ready"}`)
	})
	return mux
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
