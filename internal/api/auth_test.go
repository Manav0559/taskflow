package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTAuth(t *testing.T) {
	const secret = "test-secret"

	passthrough := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // distinctive marker: proves next.ServeHTTP ran
	})
	handler := jwtAuth(secret)(passthrough)

	validToken, err := MintToken(secret, "test-subject", time.Hour)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	wrongSecretToken, err := MintToken("some-other-secret", "test-subject", time.Hour)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	expiredToken, err := MintToken(secret, "test-subject", -time.Hour)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"malformed header, no Bearer prefix", "just-a-token", http.StatusUnauthorized},
		{"Bearer prefix but empty token", "Bearer ", http.StatusUnauthorized},
		{"garbage token", "Bearer not-a-real-jwt", http.StatusUnauthorized},
		{"wrong signing secret", "Bearer " + wrongSecretToken, http.StatusUnauthorized},
		{"expired token", "Bearer " + expiredToken, http.StatusUnauthorized},
		{"valid token", "Bearer " + validToken, http.StatusTeapot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
