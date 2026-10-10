package termix

import (
	"errors"
	"fmt"
)

// APIError is the typed form of the backend's {error:{code,message,details}} envelope.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d %s] %s", e.Status, e.Code, e.Message)
}

// NetworkError wraps transport-level failures (timeout, connection).
type NetworkError struct {
	Msg string
	Err error
}

func (e *NetworkError) Error() string { return e.Msg }
func (e *NetworkError) Unwrap() error { return e.Err }

// ChainError is raised for signing/broadcast misuse (e.g. autoSign without a signer).
type ChainError struct{ Msg string }

func (e *ChainError) Error() string { return e.Msg }

func mapAPIError(status int, body map[string]any) *APIError {
	code, message := "UNKNOWN", fmt.Sprintf("HTTP %d", status)
	var details any
	if status >= 500 {
		code = "INTERNAL_ERROR"
	}
	if body != nil {
		if e, ok := body["error"].(map[string]any); ok {
			if c, ok := e["code"].(string); ok {
				code = c
			}
			if m, ok := e["message"].(string); ok {
				message = m
			}
			details = e["details"]
		}
	}
	return &APIError{Status: status, Code: code, Message: message, Details: details}
}

func statusOf(err error) int {
	var a *APIError
	if errors.As(err, &a) {
		return a.Status
	}
	return 0
}

// Classification helpers.
func IsNotFound(err error) bool   { return statusOf(err) == 404 }
func IsValidation(err error) bool { s := statusOf(err); return s == 400 || s == 422 }
func IsAuth(err error) bool       { s := statusOf(err); return s == 401 || s == 403 }
func IsConflict(err error) bool   { return statusOf(err) == 409 }
