package termix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type routeFn func(n int) (int, any)

func testServer(routes map[string]routeFn) (*httptest.Server, map[string]int) {
	counts := map[string]int{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		mu.Lock()
		counts[key]++
		n := counts[key]
		mu.Unlock()
		fn, ok := routes[key]
		if !ok {
			w.WriteHeader(599)
			return
		}
		status, body := fn(n)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
	return srv, counts
}

func errBody(code, msg string) any { return map[string]any{"error": map[string]any{"code": code, "message": msg}} }

func TestErrorMapping(t *testing.T) {
	srv, _ := testServer(map[string]routeFn{
		"GET /api/v1/x":  func(n int) (int, any) { return 404, errBody("NOT_FOUND", "no") },
		"GET /api/v1/y":  func(n int) (int, any) { return 400, errBody("BAD", "bad") },
		"POST /api/v1/z": func(n int) (int, any) { return 409, errBody("CONFLICT", "c") },
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, MaxRetries: 0})
	ctx := context.Background()
	if _, err := apiGet[JSON](ctx, c, "/x", nil, true); !IsNotFound(err) {
		t.Errorf("want NotFound, got %v", err)
	}
	if _, err := apiGet[JSON](ctx, c, "/y", nil, true); !IsValidation(err) {
		t.Errorf("want Validation, got %v", err)
	}
	if _, err := apiPost[JSON](ctx, c, "/z", nil, true); !IsConflict(err) {
		t.Errorf("want Conflict, got %v", err)
	}
}

func TestRetryGetOn500(t *testing.T) {
	srv, counts := testServer(map[string]routeFn{
		"GET /api/v1/flaky": func(n int) (int, any) {
			if n < 3 {
				return 500, errBody("X", "x")
			}
			return 200, map[string]any{"ok": true}
		},
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, MaxRetries: 3})
	if _, err := apiGet[JSON](context.Background(), c, "/flaky", nil, true); err != nil {
		t.Fatal(err)
	}
	if counts["GET /api/v1/flaky"] != 3 {
		t.Errorf("attempts = %d, want 3", counts["GET /api/v1/flaky"])
	}
}

func TestNoRetryPostOn500(t *testing.T) {
	srv, counts := testServer(map[string]routeFn{
		"POST /api/v1/write": func(n int) (int, any) { return 500, errBody("X", "x") },
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, MaxRetries: 3})
	if _, err := apiPost[JSON](context.Background(), c, "/write", nil, false); err == nil {
		t.Fatal("expected error")
	}
	if counts["POST /api/v1/write"] != 1 {
		t.Errorf("attempts = %d, want 1", counts["POST /api/v1/write"])
	}
}

func TestDeleteTargetsPath(t *testing.T) {
	srv, counts := testServer(map[string]routeFn{
		"DELETE /api/v1/saved-listings/abc": func(n int) (int, any) { return 204, nil },
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL})
	if _, err := apiReq[any](context.Background(), c, "DELETE", "/saved-listings/abc", reqOpts{idempotent: true}); err != nil {
		t.Fatal(err)
	}
	if counts["DELETE /api/v1/saved-listings/abc"] != 1 {
		t.Errorf("delete did not target the path")
	}
}

func TestRefreshOn401(t *testing.T) {
	srv, counts := testServer(map[string]routeFn{
		"POST /api/v1/auth/refresh": func(n int) (int, any) { return 200, map[string]any{"token": "new"} },
		"GET /api/v1/secure": func(n int) (int, any) {
			if n < 2 {
				return 401, errBody("U", "no")
			}
			return 200, map[string]any{"ok": true}
		},
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, RefreshToken: "r"})
	if _, err := apiGet[JSON](context.Background(), c, "/secure", nil, false); err != nil {
		t.Fatal(err)
	}
	if c.token() != "new" {
		t.Errorf("access not refreshed: %q", c.token())
	}
	if counts["GET /api/v1/secure"] != 2 {
		t.Errorf("secure attempts = %d, want 2", counts["GET /api/v1/secure"])
	}
}
