package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_BurstThenDeny(t *testing.T) {
	rl := newRateLimiter(1, 3) // 1 token/sec refill, burst capacity 3

	for i := 0; i < 3; i++ {
		if !rl.allow("client-a") {
			t.Fatalf("request %d within burst should be allowed", i+1)
		}
	}
	if rl.allow("client-a") {
		t.Fatal("request beyond burst capacity should be denied")
	}
}

func TestRateLimiter_RefillOverTime(t *testing.T) {
	rl := newRateLimiter(10, 1) // 10 tokens/sec, burst 1

	if !rl.allow("client-b") {
		t.Fatal("first request should be allowed")
	}
	if rl.allow("client-b") {
		t.Fatal("immediate second request should be denied (bucket just emptied)")
	}

	// Backdate lastRefill to simulate elapsed time without a real sleep, exercising
	// the same lazy-refill-on-access arithmetic allow() uses on a live clock.
	rl.mu.Lock()
	rl.buckets["client-b"].lastRefill = time.Now().Add(-200 * time.Millisecond)
	rl.mu.Unlock()

	if !rl.allow("client-b") {
		t.Fatal("request after simulated 200ms at 10rps (2 tokens) should be allowed")
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	rl := newRateLimiter(1, 1)

	if !rl.allow("client-c") {
		t.Fatal("client-c's first request should be allowed")
	}
	if !rl.allow("client-d") {
		t.Fatal("client-d has its own bucket and should not be affected by client-c's usage")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"203.0.113.5:54321", "203.0.113.5"},
		{"[::1]:12345", "::1"},
		{"not-a-host-port", "not-a-host-port"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remoteAddr
		if got := clientIP(req); got != tt.want {
			t.Errorf("clientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}

func TestRateLimiterMiddleware_TooManyRequests(t *testing.T) {
	rl := newRateLimiter(1, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := rl.middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.RemoteAddr = "198.51.100.1:1111"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request (burst exhausted): status = %d, want 429", rec2.Code)
	}
}
