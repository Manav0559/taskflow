package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtAuth is a single shared-secret admin credential, not a multi-tenant identity
// system: any bearer token signed with jwtSecret grants the same full access to the
// whole API. That's a deliberate scope boundary for this admin-facing service, not an
// oversight — per-subject scopes/revocation would replace this middleware wholesale
// rather than extend it, if taskflow ever grew real multi-tenant users.
func jwtAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) || strings.TrimSpace(header[len(prefix):]) == "" {
				writeError(w, http.StatusUnauthorized, "missing or malformed authorization header")
				return
			}
			tokenStr := strings.TrimSpace(header[len(prefix):])

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("unexpected signing method")
				}
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MintToken issues an HS256 token for subject valid for ttl. There is no login
// endpoint on this API (it's a shared-secret admin service), so this is the helper
// docs/integration tests/an operator CLI use to produce a token for testing or scripted
// access.
func MintToken(secret, subject string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
