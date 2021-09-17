package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type status string

const (
	StatusSuccess status = "success"
	StatusError   status = "error"
)

type ErrorType string

const (
	ErrorTimeout      ErrorType = "timeout"
	ErrorCanceled     ErrorType = "canceled"
	ErrorExec         ErrorType = "execution"
	ErrorBadData      ErrorType = "bad_data"
	ErrorInternal     ErrorType = "internal"
	ErrorNoPermission ErrorType = "no_permission"
)

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType ErrorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

type ApiError struct {
	Typ ErrorType
	Err error
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("%s: %s", e.Typ, e.Err)
}

func RespondError(w http.ResponseWriter, apiErr *ApiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	var code int
	switch apiErr.Typ {
	case ErrorBadData:
		code = http.StatusBadRequest
	case ErrorExec:
		code = 422
	case ErrorCanceled, ErrorTimeout:
		code = http.StatusServiceUnavailable
	case ErrorInternal:
		code = http.StatusInternalServerError
	case ErrorNoPermission:
		code = http.StatusUnauthorized
	default:
		code = http.StatusInternalServerError
	}
	w.WriteHeader(code)

	_ = json.NewEncoder(w).Encode(&response{
		Status:    StatusError,
		ErrorType: apiErr.Typ,
		Error:     apiErr.Err.Error(),
		Data:      data,
	})
}
