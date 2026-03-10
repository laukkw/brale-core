package runtimeapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleDashboardOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	selectedSymbol, cards, useErr := newPortfolioUsecase(s).buildDashboardOverview(ctx, r.URL.Query().Get("symbol"))
	if useErr != nil {
		writeError(ctx, w, useErr.Status, useErr.Code, useErr.Message, useErr.Details)
		return
	}
	accountPnL, ok := newPortfolioUsecase(s).buildDashboardAccountPnL(ctx)
	var accountPnLPtr *DashboardPnLCard
	if ok {
		accountPnLPtr = &accountPnL
	}
	writeJSON(w, DashboardOverviewResponse{
		Status:     "ok",
		Symbol:     selectedSymbol,
		Symbols:    cards,
		AccountPnL: accountPnLPtr,
		Summary:    dashboardContractSummary,
		RequestID:  requestIDFromContext(ctx),
	})
}

func (s *Server) handleDashboardAccountSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	resp, useErr := newDashboardAccountUsecase(s).build(ctx)
	if useErr != nil {
		writeError(ctx, w, useErr.Status, useErr.Code, useErr.Message, useErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleDashboardKline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	if interval == "" {
		writeError(ctx, w, http.StatusBadRequest, "interval_required", "interval 不能为空", nil)
		return
	}
	limit, ok := parseBoundedPositiveInt(r.URL.Query().Get("limit"), dashboardKlineDefaultLimit, dashboardKlineMaxLimit)
	if !ok {
		writeError(ctx, w, http.StatusBadRequest, "invalid_limit", "limit 非法或超出范围", map[string]any{"max_limit": dashboardKlineMaxLimit})
		return
	}
	resp, useErr := newDashboardKlineUsecase(s).build(ctx, symbol, interval, limit)
	if useErr != nil {
		writeError(ctx, w, useErr.Status, useErr.Code, useErr.Message, useErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleDashboardDecisionFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	resp, useErr := newDashboardFlowUsecase(s).build(ctx, symbol)
	if useErr != nil {
		writeError(ctx, w, useErr.Status, useErr.Code, useErr.Message, useErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleDashboardDecisionHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	limit, ok := parseBoundedPositiveInt(r.URL.Query().Get("limit"), dashboardHistoryDefaultLimit, dashboardHistoryMaxLimit)
	if !ok {
		writeError(ctx, w, http.StatusBadRequest, "invalid_limit", "limit 非法或超出范围", map[string]any{"max_limit": dashboardHistoryMaxLimit})
		return
	}
	resp, useErr := newDashboardHistoryUsecase(s).build(ctx, symbol, limit, r.URL.Query().Get("snapshot_id"))
	if useErr != nil {
		writeError(ctx, w, useErr.Status, useErr.Code, useErr.Message, useErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func parseBoundedPositiveInt(raw string, def, max int) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return def, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	if parsed <= 0 || parsed > max {
		return 0, false
	}
	return parsed, true
}
