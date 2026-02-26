// 本文件主要内容：执行端鉴权处理（Basic 或 X-API-KEY/SECRET）。

package execution

import (
	"net/http"
	"strings"
)

const (
	ExecAuthHeader = "header"
	ExecAuthBasic  = "basic"
)

func applyExecAuth(req *http.Request, authType, apiKey, apiSecret string) {
	if req == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(authType))
	switch mode {
	case ExecAuthBasic:
		if apiKey == "" && apiSecret == "" {
			return
		}
		req.SetBasicAuth(apiKey, apiSecret)
	default:
		if apiKey != "" {
			req.Header.Set("X-API-KEY", apiKey)
		}
		if apiSecret != "" {
			req.Header.Set("X-API-SECRET", apiSecret)
		}
	}
}
