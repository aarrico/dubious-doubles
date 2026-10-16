package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		name, path, method, body string
		dbErr                    error
		status                   int
		calls                    int
	}{
		{"live without database", "/healthz", "GET", "{\"status\":\"ok\"}\n", errors.New("database password"), 200, 0},
		{"ready", "/readyz", "GET", "{\"status\":\"ready\"}\n", nil, 200, 1},
		{"unavailable", "/readyz", "GET", "{\"status\":\"unavailable\"}\n", errors.New("database password"), 503, 1},
		{"method", "/readyz", "POST", "", nil, 405, 0},
		{"unknown", "/api/v1/teams", "GET", "", nil, 404, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := Handler(func(ctx context.Context) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing database deadline")
				}
				return tc.dbErr
			}, time.Second)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status || calls != tc.calls {
				t.Fatalf("status=%d calls=%d", w.Code, calls)
			}
			if tc.body != "" && (w.Body.String() != tc.body || w.Header().Get("Content-Type") != "application/json") {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
		})
	}
}

func TestReadinessDeadline(t *testing.T) {
	h := Handler(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 10*time.Millisecond)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", w.Code)
	}
}
