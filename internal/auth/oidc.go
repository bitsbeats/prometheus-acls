package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/gorilla/sessions"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/bitsbeats/prometheus-acls/internal/config"
	"github.com/bitsbeats/prometheus-acls/internal/core"
	"github.com/bitsbeats/prometheus-acls/internal/prom"
)

type (
	// OidcAuth provides middleware and oauth handlers for authentification
	OidcAuth struct {
		loginURL    string
		redirectURL string
		cfg         *config.Config

		store       *sessions.CookieStore
		oauthConfig *oauth2.Config
		verifier    *oidc.IDTokenVerifier
		authMap     *sync.Map
	}
)

// NewAuth creates a new Auth
func NewOauthAuth(cfg *config.Config, authPath string) (a *OidcAuth, err error) {
	a = &OidcAuth{}
	a.cfg = cfg

	// urls
	a.loginURL = cfg.URL + path.Join(authPath, "login")
	a.redirectURL = cfg.URL + path.Join(authPath, "callback")

	// cookie
	a.store = sessions.NewCookieStore(cfg.CookieSecret)

	// oidc
	provider, err := oidc.NewProvider(context.Background(), cfg.OidcIssuer)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to oidc provider: %s", err)
	}
	a.oauthConfig = &oauth2.Config{
		ClientID:     cfg.OidcClientID,
		ClientSecret: cfg.OidcClientSecret,
		RedirectURL:  a.redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID},
	}
	oidcConfig := &oidc.Config{ClientID: a.oauthConfig.ClientID}
	a.verifier = provider.Verifier(oidcConfig)

	// token store (with cleanup)
	a.authMap = &sync.Map{}
	tick := time.NewTicker(10 * time.Minute)
	go func() {
		for range tick.C {
			a.authMap.Range(func(key, value interface{}) (cont bool) {
				cont = true
				token, ok := value.(*oauth2.Token)
				if !ok {
					log.WithError(fmt.Errorf(
						"%T is not a *Token", value),
					).Error("unable to cleanup token map")
					return
				}
				if time.Since(token.Expiry) > time.Hour {
					log.Debugf("cleaning orphan token for %v", key)
					a.authMap.Delete(key)
				}
				return true
			})
		}
	}()

	return
}

// auth loads the AccessToken for the current request
func (a OidcAuth) auth(r *http.Request) (t *oidc.IDToken, err error) {
	authHeader, ok := r.Header["Authorization"]
	if ok {
		// grafanas auth header is preferred
		auth := authHeader[0]
		rawIDToken := strings.TrimPrefix(auth, "Bearer ")
		t, err = a.verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			return nil, fmt.Errorf("unable to verify oidc token: %s", err)
		}
	} else {
		// otherwise use own auth
		session, _ := a.store.Get(r, SESSIONNAME)

		subject, ok := session.Values["subject"].(string)
		if !ok {
			return nil, fmt.Errorf("unable to load subject from session storage: subject not in sesson")
		}
		tokenLoader, ok := a.authMap.Load(subject)
		if !ok {
			return nil, fmt.Errorf("unable to load token from authMap: no session for subject")
		}
		token := tokenLoader.(*oauth2.Token)
		if !token.Valid() {
			var err error
			token, err = a.oauthConfig.TokenSource(r.Context(), token).Token()
			if err != nil {
				return nil, fmt.Errorf("unable to refresh token: %s", err)
			}
		}
		rawIDToken := token.AccessToken
		t, err = a.verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			return nil, fmt.Errorf("unable to verify oidc token: %s", err)
		}

	}
	return
}

func (a OidcAuth) loadACL(idToken *oidc.IDToken) (acl core.ACL, err error) {
	// add auth to context
	var claimsLoader interface{}
	err = idToken.Claims(&claimsLoader)
	if err != nil {
		return nil, err
	}
	claimsMap, ok := claimsLoader.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to cast access token claims")
	}
	roles, ok := claimsMap[a.cfg.OidcRolesClaim].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to load acces token claims from %s", a.cfg.OidcRolesClaim)
	}
	for _, role := range roles {
		var ok bool
		role, ok := role.(string)
		if !ok {
			continue
		}
		acl, ok = a.cfg.ACLMap.GetACL(role)
		if ok {
			break
		}
	}
	if acl == nil {
		acl = a.cfg.ACLMap.GetDenyACL()
	}
	return
}

// redirectErrorHandler redirects the user to a.loginURL and logs the message
func (a OidcAuth) redirectErrorHandler(w http.ResponseWriter, r *http.Request, msg string, code int) {
	http.Redirect(w, r, a.loginURL, http.StatusTemporaryRedirect)
	return
}

// LoginHandler is the HTTP route for the login
func (a OidcAuth) LoginHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := a.store.Get(r, SESSIONNAME)
	stateBytes := make([]byte, 32)
	_, err := rand.Read(stateBytes)
	if err != nil {
		log.WithError(err).Error("unable to generate oauth state")
		http.Error(w, "unable to generate oauth state", http.StatusInternalServerError)
		return
	}
	store := base64.StdEncoding.EncodeToString(stateBytes)
	session.Values["state"] = store
	err = session.Save(r, w)
	if err != nil {
		log.WithError(err).Error("unable to store session")
		http.Error(w, "unable to store session", http.StatusInternalServerError)
		return
	}

	url := a.oauthConfig.AuthCodeURL(store)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// CallbackHandler is the HTTP route for callback
func (a OidcAuth) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := a.store.Get(r, SESSIONNAME)
	wantState, ok := session.Values["state"].(string)
	if !ok {
		http.Error(w, "state missing in session", http.StatusForbidden)
		return
	}
	if r.FormValue("state") != wantState {
		http.Error(w, "oauth state invalid", http.StatusForbidden)
		return
	}
	code := r.FormValue("code")
	token, err := a.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.WithError(err).Error("unable to exchange oauth token")
		http.Error(w, "unable to exchange oauth token", http.StatusForbidden)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "id_token missing", http.StatusForbidden)
		return
	}
	idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "unable to verify id_token", http.StatusForbidden)
		return
	}
	session.Values["subject"] = idToken.Subject
	a.authMap.Store(idToken.Subject, token)
	err = session.Save(r, w)
	if err != nil {
		log.WithError(err).Info("unable to save session")
		http.Error(w, "unable to save session", http.StatusForbidden)
		return
	}

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// Middleware verifies the token and redirects to login
func (a OidcAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idToken, err := a.auth(r)
		if err != nil {
			prom.SendError(w, r, err.Error(), http.StatusBadRequest, a.redirectErrorHandler)
			return
		}
		acl, err := a.loadACL(idToken)
		if err != nil {
			log.WithError(err).Error("unable to load acl")
			prom.SendError(w, r, "unable to load acl", http.StatusBadRequest, a.redirectErrorHandler)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), "acl", acl))

		// handle request
		next.ServeHTTP(w, r)
	})
}
