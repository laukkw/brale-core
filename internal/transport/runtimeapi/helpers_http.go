package runtimeapi

import (
	"context"
	"fmt"
	"net/http"
)

func ensureMethod(ctx context.Context, w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	writeError(ctx, w, http.StatusMethodNotAllowed, "method_not_allowed", methodNotAllowedMessage(method), nil)
	return false
}

func methodNotAllowedMessage(method string) string {
	return fmt.Sprintf("仅支持 %s", method)
}
