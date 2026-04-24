package runtimeapi

import (
	"net/http"
	"strings"
	"time"

	"brale-core/internal/market"
)

func (s *Server) handleMarketStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "missing_symbol", "symbol query parameter is required", nil)
		return
	}

	inspector, ok := s.PriceSource.(market.PriceStreamInspector)
	resp := map[string]any{
		"request_id": requestIDFromContext(ctx),
		"symbol":     symbol,
	}
	foundPrice := false
	if ok {
		ss, found := inspector.StreamStatus(symbol)
		if found {
			foundPrice = true
			resp["status"] = "ok"
			resp["source"] = ss.Source
			resp["ws_connected"] = ss.Connected
			resp["last_mark_price"] = ss.LastPrice
			resp["age_ms"] = ss.AgeMs
			resp["fresh"] = ss.Fresh
			if !ss.LastPriceTS.IsZero() {
				resp["last_mark_ts"] = ss.LastPriceTS.Format(time.RFC3339Nano)
			}
		}
	}
	if s.LiquidationInspector != nil {
		if liq, found := s.LiquidationInspector.LiquidationStreamStatus(symbol); found {
			if !foundPrice {
				resp["status"] = "ok"
			}
			resp["liquidation"] = map[string]any{
				"symbol":             liq.Symbol,
				"source":             liq.Source,
				"status":             liq.Status,
				"stream_connected":   liq.StreamConnected,
				"shard_count":        liq.ShardCount,
				"coverage_sec":       liq.CoverageSec,
				"sample_count":       liq.SampleCount,
				"last_event_age_sec": liq.LastEventAgeSec,
				"complete":           liq.Complete,
			}
		}
	}
	if _, ok := resp["status"]; !ok {
		if s.PriceSource == nil {
			resp["status"] = "unsupported"
			resp["message"] = "price source does not support stream inspection"
		} else {
			resp["status"] = "not_found"
			resp["message"] = "no stream data for this symbol"
		}
	}
	writeJSON(w, resp)
}
