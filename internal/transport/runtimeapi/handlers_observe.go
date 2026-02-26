package runtimeapi

import (
	"encoding/json"
	"net/http"

	"brale-core/internal/runtime"
)

func (s *Server) handleObserveRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req observeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
		return
	}
	symbol := runtime.NormalizeSymbol(req.Symbol)
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	resp, ucErr := newObserveUsecase(s).submit(ctx, symbol)
	if ucErr != nil {
		writeError(ctx, w, ucErr.Status, ucErr.Code, ucErr.Message, ucErr.Details)
		return
	}
	writeJSON(w, resp)
}

func buildObserveResponse(res ObserveSymbolResult, requestID, traceID string) observeResponse {
	gate := res.Gate
	var inPosition map[string]any
	providerPayload := buildProviderPayload(res)
	if res.InPositionEvaluated {
		inPosition = buildInPositionPayload(res)
		providerPayload = buildInPositionProviderPayload(res)
	}
	report, reportMarkdown, reportHTML := buildObserveReport(res)
	return observeResponse{
		Symbol:         res.Symbol,
		Status:         "ok",
		Agent:          buildAgentPayload(res),
		Provider:       providerPayload,
		Gate:           buildGatePayload(gate),
		InPosition:     inPosition,
		Report:         report,
		ReportMarkdown: reportMarkdown,
		ReportHTML:     reportHTML,
		Summary:        buildObserveSummary(gate),
		RequestID:      requestID,
		SkippedExec:    true,
		TraceID:        traceID,
	}
}

func (s *Server) handleObserveReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := runtime.NormalizeSymbol(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	resp, ucErr := newObserveUsecase(s).report(ctx, symbol)
	if ucErr != nil {
		writeError(ctx, w, ucErr.Status, ucErr.Code, ucErr.Message, ucErr.Details)
		return
	}
	writeJSON(w, resp)
}
