package runtimeapi

import (
	"net/http"
	"strings"

	"brale-core/internal/runtime"
)

func (s *Server) handleTradeHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	if s.ExecClient == nil {
		writeError(ctx, w, http.StatusInternalServerError, "exec_client_missing", "ExecClient 未配置", nil)
		return
	}
	rawSymbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	normalizedSymbol := runtime.NormalizeSymbol(rawSymbol)
	if rawSymbol != "" {
		if normalizedSymbol == "" || !isValidDashboardSymbol(normalizedSymbol) {
			writeError(ctx, w, http.StatusBadRequest, "invalid_symbol", "symbol 非法", rawSymbol)
			return
		}
		if s.AllowSymbol != nil && !s.AllowSymbol(normalizedSymbol) {
			writeError(ctx, w, http.StatusBadRequest, "symbol_not_allowed", "symbol 不在允许列表", normalizedSymbol)
			return
		}
	}
	items, err := newPortfolioUsecase(s).buildTradeHistory(ctx, 10, 0, normalizedSymbol)
	if err != nil {
		writeError(ctx, w, http.StatusBadGateway, "freqtrade_trades_failed", "freqtrade trades 获取失败", err)
		return
	}
	writeJSON(w, TradeHistoryResponse{
		Status:    "ok",
		Trades:    items,
		Summary:   "",
		RequestID: requestIDFromContext(ctx),
	})
}
