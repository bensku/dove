package admin

import (
	"net/http"
	"slices"
)

type authMiddleware struct {
	handler      http.Handler
	acceptedKeys []string
}

func withAuth(handler http.Handler, acceptedKeys []string) http.Handler {
	return &authMiddleware{handler: handler, acceptedKeys: acceptedKeys}
}

func (auth *authMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if header == "" {
		header = r.Header.Get("X-Api-Key") // acme-dns API compatibility
	}
	if header == "" {
		http.Error(w, "missing API key", http.StatusUnauthorized)
		return
	}
	key := header
	if !slices.Contains(auth.acceptedKeys, key) {
		http.Error(w, "invalid API key", http.StatusForbidden)
		return
	}

	auth.handler.ServeHTTP(w, r)
}
