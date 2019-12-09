package auth

import (
	"fmt"
	"net/http"

	"github.com/bitsbeats/prometheus-acls/internal/config"
)

// SESSIONNAME is the gorilla/sessions name
const SESSIONNAME = "promacl-auth"

type (
	// Auth provides all methods neded for an auth provider
	Auth interface {
		LoginHandler(w http.ResponseWriter, r *http.Request)
		CallbackHandler(w http.ResponseWriter, r *http.Request)
		Middleware(next http.Handler) http.Handler
	}
)

// NewAuth create a new instance of the configured AuthProvider
func NewAuth(cfg *config.Config, authPath string) (a Auth, err error) {
	switch cfg.AuthProvider {
	case "oidc":
		return NewOauthAuth(cfg, authPath)
	}
	return nil, fmt.Errorf("unable to find auth provider")
}
