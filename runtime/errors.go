package runtime

import "errors"

var (
	// ErrNoProvider means no provider was configured or selected.
	ErrNoProvider = errors.New("runtime: no provider selected")
	// ErrNoFallback means provider fallback is unavailable.
	ErrNoFallback = errors.New("runtime: no fallback provider")
	// ErrMaxTurns means the run stopped before a final answer.
	ErrMaxTurns = errors.New("runtime: max turns reached")
	// ErrNilStream means the provider returned no stream for a streaming request.
	ErrNilStream = errors.New("runtime: provider returned nil stream")
)

// RuntimeError is a structured error payload for event consumers.
type RuntimeError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Cause   string `json:"cause,omitempty"`
}

func runtimeError(code string, err error) *RuntimeError {
	if err == nil {
		return nil
	}
	return &RuntimeError{Code: code, Message: err.Error(), Cause: err.Error()}
}
