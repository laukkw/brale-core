package market

import "brale-core/internal/snapshot"

type CVDMetrics struct {
	Value      float64
	Momentum   float64
	Normalized float64
	Divergence string
	PeakFlip   string
}

// =====================
// CVD 计算：基于 taker_buy 与 taker_sell 的累积差值。
// 示例：买入量>卖出量 -> CVD 上升；买入量<卖出量 -> CVD 下降。
// =====================

func ComputeCVD(candles []snapshot.Candle) (CVDMetrics, bool) {
	if len(candles) == 0 {
		return CVDMetrics{}, false
	}
	cvd := make([]float64, 0, len(candles))
	closes := make([]float64, 0, len(candles))
	sum := 0.0
	for _, c := range candles {
		sum += c.TakerBuyVolume - c.TakerSellVolume
		cvd = append(cvd, sum)
		closes = append(closes, c.Close)
	}
	last := cvd[len(cvd)-1]
	momentum := 0.0
	if len(cvd) > 6 {
		momentum = last - cvd[len(cvd)-6]
	}
	minVal, maxVal := cvd[0], cvd[0]
	for _, v := range cvd[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	norm := 0.5
	if maxVal > minVal {
		norm = (last - minVal) / (maxVal - minVal)
	}
	priceNow := closes[len(closes)-1]
	pricePrev := closes[0]
	cvdPrev := cvd[0]
	if len(closes) > 6 {
		pricePrev = closes[len(closes)-6]
		cvdPrev = cvd[len(cvd)-6]
	}
	divergence := "neutral"
	if priceNow > pricePrev && last < cvdPrev {
		divergence = "down"
	} else if priceNow < pricePrev && last > cvdPrev {
		divergence = "up"
	}
	peakFlip := "none"
	if len(cvd) > 3 {
		a, b, c := cvd[len(cvd)-1], cvd[len(cvd)-2], cvd[len(cvd)-3]
		if a < b && b > c {
			peakFlip = "local_top"
		} else if a > b && b < c {
			peakFlip = "local_bottom"
		}
	}
	return CVDMetrics{
		Value:      last,
		Momentum:   momentum,
		Normalized: norm,
		Divergence: divergence,
		PeakFlip:   peakFlip,
	}, true
}
