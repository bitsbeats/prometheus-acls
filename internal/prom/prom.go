package prom

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type (
	// Error Struct for a Prometheus error
	Error struct {
		Status    string                 `json:"status"`
		Data      map[string]interface{} `json:"datay"`
		ErrorType string                 `json:"error_type"`
		Error     string                 `json:"error"`
		Warnings  []string               `json:"warnings"`
	}

	// NotPromHandlerFunc is a function that is called if the error message
	// is not displayed in Prometheus
	NotPromHandlerFunc func(w http.ResponseWriter, r *http.Request, msg string, code int)
)

// SendError sends a Prometheus compatible error message and logs
func SendError(w http.ResponseWriter, r *http.Request, msg string, code int, notPromHandler NotPromHandlerFunc) {
	if r.URL.EscapedPath() == "/api/v1/query" || r.URL.EscapedPath() == "/api/v1/query_range" {
		p := Error{
			Status:    "error",
			Data:      map[string]interface{}{},
			ErrorType: "server_error",
			Error:     msg,
			Warnings:  []string{},
		}
		w.WriteHeader(code)
		err := json.NewEncoder(w).Encode(p)
		if err != nil {
			log.Printf("unable to send prometheus error: %s", err)
		}
	} else {
		if notPromHandler != nil {
			notPromHandler(w, r, msg, code)
		} else {
			http.Error(w, msg, code)
		}
	}
}
