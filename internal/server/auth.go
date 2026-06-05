package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Authorized checks whether the request carries a valid Bearer token
// matching the expected value. The comparison is constant-time-safe
// via crypto/subtle.ConstantTimeCompare to avoid timing side-channels.
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

	actual := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(actual), []byte(token)) == 1
}
