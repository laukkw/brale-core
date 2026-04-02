package dashboard

import (
	"math"
	"time"

	"brale-core/internal/execution"
	symbolpkg "brale-core/internal/pkg/symbol"
)

const (
	PNLRealizedSourceRealizedProfit = "realized_profit"
	PNLRealizedSourceCloseProfitAbs = "close_profit_abs"
	PNLUnrealizedSourceProfitAbs    = "profit_abs"
	PNLTotalSourceTotalProfitAbs    = "total_profit_abs"
	PNLTotalSourceComponents        = "realized_plus_unrealized"

	PNLDriftThreshold = 0.01
)

type PnLCard struct {
	Realized   float64
	Unrealized float64
	Total      float64
}

type PnLProvenance struct {
	RealizedSource   string
	UnrealizedSource string
	TotalSource      string
}

type Reconciliation struct {
	Status         string
	DriftAbs       float64
	DriftPct       float64
	DriftThreshold float64
}

func ResolvePnLFromTrade(tr execution.Trade) (PnLCard, PnLProvenance) {
	realized := float64(tr.RealizedProfit)
	realizedSource := PNLRealizedSourceRealizedProfit
	if realized == 0 {
		fallback := float64(tr.CloseProfitAbs)
		if fallback != 0 {
			realized = fallback
			realizedSource = PNLRealizedSourceCloseProfitAbs
		}
	}

	unrealized := float64(tr.ProfitAbs)
	total := float64(tr.TotalProfitAbs)
	totalSource := PNLTotalSourceTotalProfitAbs
	if total == 0 {
		total = realized + unrealized
		totalSource = PNLTotalSourceComponents
	}

	return PnLCard{Realized: realized, Unrealized: unrealized, Total: total}, PnLProvenance{
		RealizedSource:   realizedSource,
		UnrealizedSource: PNLUnrealizedSourceProfitAbs,
		TotalSource:      totalSource,
	}
}

func ReconcilePnL(pnl PnLCard) Reconciliation {
	expectedTotal := pnl.Realized + pnl.Unrealized
	driftAbs := math.Abs(pnl.Total - expectedTotal)
	base := math.Max(math.Abs(expectedTotal), math.Abs(pnl.Total))
	driftPct := 0.0
	if base > 0 {
		driftPct = driftAbs / base
	}

	status := "ok"
	if driftAbs > PNLDriftThreshold {
		status = "warn"
	}
	if driftAbs > PNLDriftThreshold*5 {
		status = "error"
	}

	return Reconciliation{
		Status:         status,
		DriftAbs:       driftAbs,
		DriftPct:       driftPct,
		DriftThreshold: PNLDriftThreshold,
	}
}

func ResolveLeverage(tr execution.Trade) float64 {
	lever := float64(tr.Leverage)
	if lever > 0 {
		return lever
	}
	notional := float64(tr.OpenRate) * float64(tr.Amount)
	margin := float64(tr.StakeAmount)
	if margin <= 0 {
		margin = float64(tr.OpenTradeValue)
	}
	if notional > 0 && margin > 0 {
		ratio := notional / margin
		if ratio > 0 {
			return ratio
		}
	}
	return 0
}

func ExtractAccountTotalProfit(quote map[string]any) (float64, bool) {
	if quote == nil {
		return 0, false
	}
	startingCapital, hasStartingCapital := execution.AsFloat(quote["starting_capital"])
	startingCapitalRatio, hasStartingCapitalRatio := execution.AsFloat(quote["starting_capital_ratio"])
	if hasStartingCapital && hasStartingCapitalRatio {
		return startingCapital * startingCapitalRatio, true
	}
	if hasStartingCapital {
		if totalBot, ok := execution.AsFloat(quote["total_bot"]); ok {
			return totalBot - startingCapital, true
		}
		if total, ok := execution.AsFloat(quote["total"]); ok {
			return total - startingCapital, true
		}
	}
	return 0, false
}

func NormalizeFreqtradePair(pair string) string {
	return symbolpkg.FromFreqtradePair(pair)
}

func ParseMillisTimestamp(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ts)
}

var shanghaiLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}()

func PositionStatusTiming(openFillTimestamp int64) (string, int64, int64) {
	if openFillTimestamp <= 0 {
		return "", 0, 0
	}
	var openedAt time.Time
	if openFillTimestamp < 1e12 {
		openedAt = time.Unix(openFillTimestamp, 0)
	} else {
		openedAt = time.UnixMilli(openFillTimestamp)
	}
	openedAtText := openedAt.In(shanghaiLocation).Format("2006-01-02 15:04:05")
	if openedAt.IsZero() {
		return "", 0, 0
	}
	now := time.Now()
	durationMin := int64(0)
	if now.After(openedAt) {
		durationMin = int64(now.Sub(openedAt).Minutes())
	}
	return openedAtText, durationMin, durationMin * 60
}
