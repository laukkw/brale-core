package execution

import (
	"fmt"

	"brale-core/internal/pkg/errclass"
)

type executionError struct {
	msg   string
	cause error
	class errclass.Classification
}

func (e executionError) Error() string {
	return e.msg
}

func (e executionError) Unwrap() error {
	return e.cause
}

func (e executionError) Classification() errclass.Classification {
	return e.class
}

func execValidationErrorf(format string, args ...any) error {
	return executionError{
		msg: fmt.Sprintf(format, args...),
		class: errclass.Classification{
			Kind:      "validation",
			Scope:     "execution",
			Retryable: false,
			Action:    "abort",
			Reason:    "invalid_request",
		},
	}
}

func execNotFoundErrorf(format string, args ...any) error {
	return executionError{
		msg: fmt.Sprintf(format, args...),
		class: errclass.Classification{
			Kind:      "validation",
			Scope:     "execution",
			Retryable: false,
			Action:    "abort",
			Reason:    "not_found",
		},
	}
}

func execExternalError(err error, reason string) error {
	if err == nil {
		return nil
	}
	return executionError{
		msg:   err.Error(),
		cause: err,
		class: errclass.Classification{
			Kind:      "external",
			Scope:     "execution",
			Retryable: true,
			Action:    "retry",
			Reason:    reason,
		},
	}
}
