package runtimeapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/asyncjob"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type observeUsecase struct {
	server *Server
}

func newObserveUsecase(s *Server) observeUsecase {
	return observeUsecase{server: s}
}

func (u observeUsecase) submit(ctx context.Context, symbol string) (observeResponse, *usecaseError) {
	if u.server == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	logger := logging.FromContext(ctx).Named("runtime-api")
	resolved, err := u.server.Resolver.Resolve(symbol)
	if err != nil {
		logger.Error("resolve symbol failed", zap.Error(err), zap.String("symbol", symbol))
		return observeResponse{}, &usecaseError{Status: 400, Code: "symbol_invalid", Message: "symbol 配置不可用", Details: err.Error()}
	}
	if resolved.Pipeline == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "pipeline_missing", Message: "运行链路不可用"}
	}
	key, err := buildObserveJobKey(resolved)
	if err != nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "job_key_failed", Message: "任务 Key 生成失败", Details: err.Error()}
	}
	jobCtx := context.WithoutCancel(ctx)
	snap, _, err := u.server.ObserveJobs.Enqueue(jobCtx, key, func(jobCtx context.Context, jobID string) (observeResponse, error) {
		return u.runObserve(jobCtx, resolved, jobID)
	})
	if err != nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "observe_enqueue_failed", Message: "观察任务提交失败", Details: err.Error()}
	}
	if snap.Status == asyncjob.StatusDone {
		resp := snap.Value
		resp.RequestID = snap.ID
		resp.TraceID = requestIDFromContext(ctx)
		return resp, nil
	}
	if snap.Status == asyncjob.StatusFailed {
		errText := "观察执行失败"
		if snap.Err != nil {
			errText = snap.Err.Error()
		}
		return observeResponse{}, &usecaseError{Status: 500, Code: "observe_failed", Message: "观察执行失败", Details: errText}
	}
	return observeResponse{
		Symbol:      resolved.Symbol,
		Status:      string(snap.Status),
		Summary:     "观察任务已提交",
		RequestID:   snap.ID,
		SkippedExec: true,
		TraceID:     requestIDFromContext(ctx),
	}, nil
}

func buildObserveJobKey(resolved ResolvedSymbol) (string, error) {
	payload := observeJobKey{Symbol: resolved.Symbol, Intervals: resolved.Intervals, KlineLimit: resolved.KlineLimit}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:]), nil
}

func (u observeUsecase) runObserve(ctx context.Context, resolved ResolvedSymbol, jobID string) (observeResponse, error) {
	logger := logging.FromContext(ctx).Named("runtime-api")
	acct := u.buildObserveAccountState(ctx, resolved.Symbol, logger)
	risk := execution.RiskParams{RiskPerTradePct: resolved.RiskPercent}
	results, err := resolved.Pipeline.RunOnceObserveAsFlat(ctx, []string{resolved.Symbol}, resolved.Intervals, resolved.KlineLimit, acct, risk)
	if err != nil {
		logger.Error("observe run failed", zap.Error(err), zap.String("symbol", resolved.Symbol))
		return observeResponse{}, err
	}
	res, ok := findSymbolResult(results, resolved.Symbol)
	if !ok {
		return observeResponse{}, fmt.Errorf("observe result missing")
	}
	resp := buildObserveResponse(res, jobID, requestIDFromContext(ctx))
	u.storeLast(resolved.Symbol, resp)
	return resp, nil
}

func (u observeUsecase) buildObserveAccountState(ctx context.Context, symbol string, logger *zap.Logger) execution.AccountState {
	acct := execution.AccountState{}
	if u.server == nil || u.server.ExecClient == nil {
		return acct
	}
	acct, err := newPortfolioUsecase(u.server).buildObserveAccountState(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("observe account fetch failed", zap.Error(err), zap.String("symbol", symbol))
		}
		return acct
	}
	return acct
}

func (u observeUsecase) storeLast(symbol string, resp observeResponse) {
	if u.server == nil {
		return
	}
	u.server.lastMu.Lock()
	defer u.server.lastMu.Unlock()
	if u.server.lastRun == nil {
		u.server.lastRun = make(map[string]lastObserve)
	}
	u.server.lastRun[symbol] = lastObserve{resp: resp, at: time.Now()}
}

func (u observeUsecase) report(ctx context.Context, symbol string) (observeResponse, *usecaseError) {
	if u.server == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	u.server.lastMu.RLock()
	defer u.server.lastMu.RUnlock()
	if u.server.lastRun == nil {
		return observeResponse{}, &usecaseError{Status: 404, Code: "report_not_found", Message: "暂无该符号观察结果"}
	}
	rec, ok := u.server.lastRun[symbol]
	if !ok {
		return observeResponse{}, &usecaseError{Status: 404, Code: "report_not_found", Message: "暂无该符号观察结果"}
	}
	resp := rec.resp
	resp.TraceID = requestIDFromContext(ctx)
	return resp, nil
}

func findSymbolResult(results []ObserveSymbolResult, symbol string) (ObserveSymbolResult, bool) {
	for _, item := range results {
		if strings.EqualFold(item.Symbol, symbol) {
			return item, true
		}
	}
	return ObserveSymbolResult{}, false
}
