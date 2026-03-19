package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Provider interface {
	Call(ctx context.Context, system, user string) (string, error)
}

type SessionCapableProvider interface {
	CallWithSession(ctx context.Context, sessionID, system, user string) (string, error)
}

type SessionCapabilityReason string

const (
	SessionCapabilityUnsupported  SessionCapabilityReason = "unsupported"
	SessionCapabilityCreateFailed SessionCapabilityReason = "create_failed"
	SessionCapabilityExpired      SessionCapabilityReason = "expired"
)

type SessionCapabilityError struct {
	Reason SessionCapabilityReason
	Err    error
}

func (e SessionCapabilityError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("session capability %s", e.Reason)
	}
	return fmt.Sprintf("session capability %s: %v", e.Reason, e.Err)
}

func (e SessionCapabilityError) Unwrap() error {
	return e.Err
}

func (e SessionCapabilityError) IsValid() bool {
	switch e.Reason {
	case SessionCapabilityUnsupported, SessionCapabilityCreateFailed, SessionCapabilityExpired:
		return true
	default:
		return false
	}
}

func NewSessionCapabilityError(reason SessionCapabilityReason, err error) error {
	wrapped := SessionCapabilityError{Reason: reason, Err: err}
	if !wrapped.IsValid() {
		return fmt.Errorf("invalid session capability reason: %q", reason)
	}
	return wrapped
}

func IsSessionCapabilityError(err error) bool {
	if err == nil {
		return false
	}
	var capabilityErr SessionCapabilityError
	if !errors.As(err, &capabilityErr) {
		return false
	}
	return capabilityErr.IsValid()
}

func IsSessionCapabilityReason(err error, reason SessionCapabilityReason) bool {
	if err == nil {
		return false
	}
	var capabilityErr SessionCapabilityError
	if !errors.As(err, &capabilityErr) {
		return false
	}
	return capabilityErr.IsValid() && capabilityErr.Reason == reason
}

func CallWithOptionalSession(ctx context.Context, provider Provider, sessionID, system, user string) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return provider.Call(ctx, system, user)
	}
	withSession, ok := provider.(SessionCapableProvider)
	if !ok {
		return "", NewSessionCapabilityError(SessionCapabilityUnsupported, fmt.Errorf("provider does not implement session calls"))
	}
	return withSession.CallWithSession(ctx, sessionID, system, user)
}
