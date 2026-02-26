package errclass

import (
	"errors"
	"fmt"
)

type Kind string
type Scope string
type Action string

const (
	KindUnknown  Kind   = "unknown"
	ScopeUnknown Scope  = "unknown"
	ActionNone   Action = "none"
)

type Classification struct {
	Kind      Kind
	Scope     Scope
	Retryable bool
	Action    Action
	Reason    string
}

var Unknown = Classification{
	Kind:      KindUnknown,
	Scope:     ScopeUnknown,
	Retryable: false,
	Action:    ActionNone,
	Reason:    "unknown",
}

type Classified interface {
	Classification() Classification
}

type ValidationError struct {
	msg    string
	cause  error
	scope  Scope
	reason string
}

func (e ValidationError) Error() string {
	return e.msg
}

func (e ValidationError) Unwrap() error {
	return e.cause
}

func (e ValidationError) Classification() Classification {
	return Classification{
		Kind:      "validation",
		Scope:     e.scope,
		Retryable: false,
		Action:    "abort",
		Reason:    e.reason,
	}
}

func ValidationErrorf(scope Scope, reason string, format string, args ...any) error {
	return ValidationError{
		msg:    fmt.Sprintf(format, args...),
		scope:  scope,
		reason: reason,
	}
}

func ClassifyError(err error) Classification {
	if err == nil {
		return Unknown
	}
	var classified Classified
	if errors.As(err, &classified) {
		return classified.Classification()
	}
	return Unknown
}
