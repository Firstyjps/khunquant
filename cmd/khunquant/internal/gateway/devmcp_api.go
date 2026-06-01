package gateway

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// generateDevMCPToken generates a cryptographically random 24-byte hex token
// for the developer MCP server bearer auth guard.
// Mirrors the launcher token generation in web/backend/main.go.
func generateDevMCPToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("devmcp: failed to generate token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// bearerTokenMiddleware rejects requests that don't carry the correct
// Authorization: Bearer <token> header. Uses constant-time comparison
// to prevent timing side-channels.
// If token is empty, all requests are allowed through (token auth disabled).
func bearerTokenMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			auth := r.Header.Get("Authorization")
			var provided string
			if strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
