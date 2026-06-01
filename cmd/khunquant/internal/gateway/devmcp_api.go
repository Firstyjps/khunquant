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

// bearerTokenMiddleware rejects requests that don't carry the correct token.
// Accepts it via Authorization: Bearer <token> header OR ?token=<token> query
// parameter, so MCP clients that can't set custom headers (e.g. Codex rmcp)
// can embed the token directly in the URL. Uses constant-time comparison.
// If token is empty, all requests are allowed through (token auth disabled).
func bearerTokenMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			var provided string
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			} else if q := r.URL.Query().Get("token"); q != "" {
				provided = q
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
