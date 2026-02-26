package runtimeapi

import (
	"encoding/json"
	"io"
	"net/http"

	"brale-core/internal/pkg/logging"
	"brale-core/internal/runtime"

	"go.uber.org/zap"
)

func (s *Server) handleScheduleEnable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx).Named("runtime-api")
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req scheduleToggleRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if err != io.EOF {
			writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
			return
		}
	}
	if req.Enable != nil && !*req.Enable {
		writeError(ctx, w, http.StatusBadRequest, "invalid_enable", "enable 必须为 true", nil)
		return
	}
	resp := newScheduleUsecase(s).enable()
	resp.RequestID = requestIDFromContext(ctx)
	logger.Info("llm schedule enabled")
	writeJSON(w, resp)
}

func (s *Server) handleScheduleDisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx).Named("runtime-api")
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req scheduleToggleRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if err != io.EOF {
			writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
			return
		}
	}
	if req.Enable != nil && *req.Enable {
		writeError(ctx, w, http.StatusBadRequest, "invalid_enable", "enable 必须为 false", nil)
		return
	}
	resp := newScheduleUsecase(s).disable(ctx, logger)
	resp.RequestID = requestIDFromContext(ctx)
	logger.Info("llm schedule disabled")
	writeJSON(w, resp)
}

func (s *Server) handleScheduleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	resp := newScheduleUsecase(s).status()
	resp.RequestID = requestIDFromContext(ctx)
	writeJSON(w, resp)
}

func (s *Server) handleScheduleSymbol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx).Named("runtime-api")
	if !ensureMethod(ctx, w, r, http.MethodPost) {
		return
	}
	var req scheduleSymbolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(ctx, w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
		return
	}
	symbol := runtime.NormalizeSymbol(req.Symbol)
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	resp, err := newScheduleUsecase(s).setSymbolMode(symbol, req.Enable)
	if err != nil {
		logger.Error("set symbol mode failed", zap.Error(err), zap.String("symbol", symbol))
		writeError(ctx, w, http.StatusInternalServerError, "set_symbol_failed", "更新符号状态失败", err.Error())
		return
	}
	resp.RequestID = requestIDFromContext(ctx)
	logger.Info("symbol mode updated", zap.String("symbol", symbol), zap.Bool("enable", req.Enable))
	writeJSON(w, resp)
}
