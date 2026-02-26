package reconcile

import (
	"fmt"

	"brale-core/internal/pkg/errclass"
)

type reconcileValidationError struct {
	msg string
}

func (e reconcileValidationError) Error() string {
	return e.msg
}

func (e reconcileValidationError) Classification() errclass.Classification {
	return errclass.Classification{
		Kind:      "validation",
		Scope:     "reconcile",
		Retryable: false,
		Action:    "abort",
		Reason:    "invalid_reconcile",
	}
}

func reconcileValidationErrorf(format string, args ...any) error {
	return reconcileValidationError{msg: fmt.Sprintf(format, args...)}
}
