package market

import (
	"errors"
	"fmt"

	"brale-core/internal/pkg/errclass"
)

var ErrPriceUnavailable = errors.New("price unavailable")

type MarketError struct {
	msg   string
	cause error
	class errclass.Classification
}

func (e MarketError) Error() string {
	return e.msg
}

func (e MarketError) Unwrap() error {
	return e.cause
}

func (e MarketError) Classification() errclass.Classification {
	return e.class
}

func ValidationErrorf(format string, args ...any) error {
	return MarketError{
		msg: fmt.Sprintf(format, args...),
		class: errclass.Classification{
			Kind:      "validation",
			Scope:     "market",
			Retryable: false,
			Action:    "abort",
			Reason:    "invalid_request",
		},
	}
}

func UnavailableErrorf(format string, args ...any) error {
	return MarketError{
		msg: fmt.Sprintf(format, args...),
		class: errclass.Classification{
			Kind:      "external",
			Scope:     "market",
			Retryable: true,
			Action:    "fallback",
			Reason:    "data_unavailable",
		},
	}
}

func ExternalError(err error, reason string) error {
	if err == nil {
		return nil
	}
	return MarketError{
		msg:   err.Error(),
		cause: err,
		class: errclass.Classification{
			Kind:      "external",
			Scope:     "market",
			Retryable: true,
			Action:    "retry",
			Reason:    reason,
		},
	}
}
