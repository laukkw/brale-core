package telegrambot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"brale-core/internal/pkg/httpclient"
)

type scheduleToggleRequest struct {
	Enable *bool `json:"enable"`
}

func (b *Bot) doRuntimeRequest(ctx context.Context, method, path string, payload any, out any) error {
	url := b.runtimeBase + path
	req, err := httpclient.NewJSONRequest(ctx, method, url, payload)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil {
			if strings.TrimSpace(apiErr.Msg) != "" {
				return errors.New(apiErr.Msg)
			}
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *Bot) fetchMonitorStatus(ctx context.Context) (MonitorStatusResponse, error) {
	var out MonitorStatusResponse
	err := b.doRuntimeRequest(ctx, http.MethodGet, "/api/runtime/monitor/status", nil, &out)
	return out, err
}

func (b *Bot) fetchPositionStatus(ctx context.Context) (PositionStatusResponse, error) {
	var out PositionStatusResponse
	err := b.doRuntimeRequest(ctx, http.MethodGet, "/api/runtime/position/status", nil, &out)
	return out, err
}

func (b *Bot) fetchTradeHistory(ctx context.Context) (TradeHistoryResponse, error) {
	var out TradeHistoryResponse
	err := b.doRuntimeRequest(ctx, http.MethodGet, "/api/runtime/position/history", nil, &out)
	return out, err
}

func (b *Bot) fetchDecisionLatest(ctx context.Context, symbol string) (DecisionLatestResponse, error) {
	var out DecisionLatestResponse
	path := "/api/runtime/decision/latest?symbol=" + url.QueryEscape(symbol)
	err := b.doRuntimeRequest(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (b *Bot) fetchNewsOverlayLatest(ctx context.Context) (NewsOverlayLatestResponse, error) {
	var out NewsOverlayLatestResponse
	err := b.doRuntimeRequest(ctx, http.MethodGet, "/api/runtime/news_overlay/latest", nil, &out)
	return out, err
}

func (b *Bot) fetchObserveReport(ctx context.Context, symbol string) (ObserveResponse, error) {
	var out ObserveResponse
	path := "/api/observe/report?symbol=" + url.QueryEscape(symbol)
	err := b.doRuntimeRequest(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (b *Bot) postScheduleToggle(ctx context.Context, enable bool) (ScheduleResponse, error) {
	var out ScheduleResponse
	req := scheduleToggleRequest{Enable: &enable}
	path := "/api/runtime/schedule/disable"
	if enable {
		path = "/api/runtime/schedule/enable"
	}
	err := b.doRuntimeRequest(ctx, http.MethodPost, path, req, &out)
	return out, err
}
