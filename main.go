package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/bitsbeats/prometheus-acls/internal/auth"
	"github.com/bitsbeats/prometheus-acls/internal/config"
	"github.com/bitsbeats/prometheus-acls/internal/labeler"
)

func main() {
	// config
	cfg, err := config.Parse()
	if err != nil {
		log.WithError(err).Fatalf("unable to load config")
	}
	// mux
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// auth
	a, err := auth.NewAuth(cfg, "/oauth/")
	if err != nil {
		log.WithError(err).Fatalf("unable to setup auth")
	}
	mux.HandleFunc("/oauth/login", a.LoginHandler)
	mux.HandleFunc("/oauth/callback", a.CallbackHandler)

	// reverse proxy
	u, err := url.Parse(cfg.PrometheusURL)
	if err != nil {
		log.WithError(err).Fatalf("unable to parse prometheus url")
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	l := labeler.NewLabeler()
	promacl := l.PromACLMiddlewareFor(u)

	// authprotect -> acls -> prometheus
	mux.Handle("/", a.Middleware(promacl(proxy)))

	// serve
	log.WithField("listen", cfg.Listen).Info("listening")
	err = http.ListenAndServe(cfg.Listen, mux)
	if err != nil {
		log.WithError(err).Fatalf("unable to start webserver")
	}
}
