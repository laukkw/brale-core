package runtimeapi

import (
	"net/http"
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
	items, err := newPortfolioUsecase(s).buildTradeHistory(ctx, 10, 0)
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
