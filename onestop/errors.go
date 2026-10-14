package onestop

import "fmt"

// APIError is returned when the API answers with a failure.
type APIError struct {
	StatusCode         int
	RequestID, Message string
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("onestop: HTTP %d (%s): %s", e.StatusCode, e.RequestID, e.Message)
	}
	return fmt.Sprintf("onestop: HTTP %d: %s", e.StatusCode, e.Message)
}
