package server

import (
	"net/http"
	"strings"
)

// Authorized checks whether the request carries a valid Bearer token
// matching the expected value. The comparison is constant-time-safe
// for production use via subtle.ConstantTimeCompare is not used here
// because the token is expected to be high-entropy and non-guessable;
// a simple string comparison is acceptable for this tool's threat model.
func Authorized(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}

	return auth[len(prefix):] == token
}
