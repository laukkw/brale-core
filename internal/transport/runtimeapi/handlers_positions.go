package runtimeapi

import (
	"context"
	"net/http"
)

func (s *Server) handlePositionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	positions, err := s.buildPositionStatus(ctx)
	if err != nil {
		writeError(ctx, w, http.StatusBadGateway, "position_status_failed", "持仓状态获取失败", err)
		return
	}
	resp := PositionStatusResponse{
		Status:    "ok",
		Positions: positions,
		Summary:   "",
		RequestID: requestIDFromContext(ctx),
	}
	writeJSON(w, resp)
}

func (s *Server) buildPositionStatus(ctx context.Context) ([]PositionStatusItem, error) {
	return newPortfolioUsecase(s).buildPositionStatus(ctx)
}
