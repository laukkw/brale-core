package runtime

import (
	"context"
	"encoding/json"

	"brale-core/internal/decision"
	"brale-core/internal/decision/features"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"
	"go.uber.org/zap"
)

type shadowComparingCompressor struct {
	primary decision.Compressor

	primaryName string
	shadowName  string

	primaryComputer decision.IndicatorComputer
	shadowComputer  decision.IndicatorComputer

	indicatorOptions   decision.IndicatorCompressOptions
	trendOptionsByInt  map[string]decision.TrendCompressOptions
	defaultTrendOption decision.TrendCompressOptions

	report    func(context.Context, features.EngineDiffReport)
	reportErr func(context.Context, features.FeatureError)
}

func (c *shadowComparingCompressor) Compress(ctx context.Context, snap snapshot.MarketSnapshot) (features.CompressionResult, []features.FeatureError, error) {
	out, errs, err := c.primary.Compress(ctx, snap)
	if err != nil {
		return features.CompressionResult{}, nil, err
	}
	for symbol, candlesByInterval := range snap.Klines {
		if len(candlesByInterval) == 0 {
			continue
		}
		report, diffErr := features.RunEngineDiff(features.EngineDiffRequest{
			Symbol:            symbol,
			BaselineName:      c.primaryName,
			Baseline:          c.primaryComputer,
			CandidateName:     c.shadowName,
			Candidate:         c.shadowComputer,
			IndicatorOptions:  c.indicatorOptions,
			TrendOptionsByInt: c.trendOptionsByInt,
			DefaultTrendOpts:  c.defaultTrendOption,
			CandlesByInterval: candlesByInterval,
		})
		if diffErr != nil {
			featureErr := features.FeatureError{
				Symbol: symbol,
				Stage:  "shadow_diff",
				Err:    diffErr,
			}
			c.emitReportErr(ctx, featureErr)
			errs = append(errs, featureErr)
			continue
		}
		c.emitReport(ctx, report)
	}
	return out, errs, nil
}

func (c *shadowComparingCompressor) emitReport(ctx context.Context, report features.EngineDiffReport) {
	if c.report != nil {
		c.report(ctx, report)
		return
	}
	raw, err := json.Marshal(report)
	if err != nil {
		logging.FromContext(ctx).Warn("indicator shadow diff marshal failed",
			zap.String("symbol", report.Symbol),
			zap.String("baseline", report.Baseline),
			zap.String("candidate", report.Candidate),
			zap.Error(err),
		)
		return
	}
	logging.FromContext(ctx).Info("indicator shadow diff",
		zap.String("symbol", report.Symbol),
		zap.String("baseline", report.Baseline),
		zap.String("candidate", report.Candidate),
		zap.ByteString("report_json", raw),
	)
}

func (c *shadowComparingCompressor) emitReportErr(ctx context.Context, featureErr features.FeatureError) {
	if c.reportErr != nil {
		c.reportErr(ctx, featureErr)
		return
	}
	logging.FromContext(ctx).Warn("indicator shadow diff failed",
		zap.String("symbol", featureErr.Symbol),
		zap.String("baseline", c.primaryName),
		zap.String("candidate", c.shadowName),
		zap.Error(featureErr.Err),
	)
}
