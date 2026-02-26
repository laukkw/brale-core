package runtimeapi

import (
	"brale-core/internal/runtime"
	"encoding/json"
	"net/http"
)

func (s *Server) handleDebugPlanInject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req debugPlanInjectRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
		return
	}
	resp, ucErr := newDebugPlanUsecase(s).inject(ctx, req)
	if ucErr != nil {
		writeError(ctx, w, ucErr.Status, ucErr.Code, ucErr.Message, ucErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleDebugPlanStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := runtime.NormalizeSymbol(r.URL.Query().Get("symbol"))
	resp, ucErr := newDebugPlanUsecase(s).status(ctx, symbol)
	if ucErr != nil {
		writeError(ctx, w, ucErr.Status, ucErr.Code, ucErr.Message, ucErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleDebugPlanClear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req debugPlanClearRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
		return
	}
	resp, ucErr := newDebugPlanUsecase(s).clear(ctx, req.Symbol)
	if ucErr != nil {
		writeError(ctx, w, ucErr.Status, ucErr.Code, ucErr.Message, ucErr.Details)
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}
