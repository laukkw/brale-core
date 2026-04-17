package logging

import (
	"fmt"

	"go.uber.org/zap"
)

type RetryableHTTPLogger struct {
	Logger *zap.Logger
}

func (l RetryableHTTPLogger) Printf(format string, args ...interface{}) {
	l.base().Info(fmt.Sprintf(format, args...))
}

func (l RetryableHTTPLogger) Error(msg string, keysAndValues ...interface{}) {
	l.base().Error(msg, kvFields(keysAndValues...)...)
}

func (l RetryableHTTPLogger) Info(msg string, keysAndValues ...interface{}) {
	l.base().Info(msg, kvFields(keysAndValues...)...)
}

func (l RetryableHTTPLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.base().Debug(msg, kvFields(keysAndValues...)...)
}

func (l RetryableHTTPLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.base().Warn(msg, kvFields(keysAndValues...)...)
}

func (l RetryableHTTPLogger) base() *zap.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return L()
}

func kvFields(keysAndValues ...interface{}) []zap.Field {
	if len(keysAndValues) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, (len(keysAndValues)+1)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key := fmt.Sprintf("arg_%d", i)
		if s, ok := keysAndValues[i].(string); ok && s != "" {
			key = s
		}
		if i+1 < len(keysAndValues) {
			fields = append(fields, zap.Any(key, keysAndValues[i+1]))
			continue
		}
		fields = append(fields, zap.Any(key, nil))
	}
	return fields
}
