package snapshot

import "time"

type Candle struct {
	OpenTime        int64
	Open            float64
	High            float64
	Low             float64
	Close           float64
	Volume          float64
	TakerBuyVolume  float64
	TakerSellVolume float64
}

type OIBlock struct {
	Value     float64
	Timestamp int64
}

type FundingBlock struct {
	Rate      float64
	Timestamp int64
}

type LSRBlock struct {
	LongShortRatio float64
	Timestamp      int64
}

type FearGreedPoint struct {
	Value     float64
	Timestamp int64
}

type LiqBlock struct {
	Volume    float64
	Timestamp int64
}

const (
	LiqWindow5m = "5m"
	LiqWindow1h = "1h"
	LiqWindow4h = "4h"
)

var DefaultLiqWindows = []string{LiqWindow5m, LiqWindow1h, LiqWindow4h}

const (
	LiqPriceBinBps25  = 25
	LiqPriceBinBps50  = 50
	LiqPriceBinBps100 = 100
	LiqPriceBinBps200 = 200
	LiqPriceBinBps400 = 400
)

var DefaultLiqPriceBinsBps = []int{
	LiqPriceBinBps25,
	LiqPriceBinBps50,
	LiqPriceBinBps100,
	LiqPriceBinBps200,
	LiqPriceBinBps400,
}

type LiqRelMetrics struct {
	VolOverOI     float64
	VolOverVolume float64
	ZScore        float64
	Spike         bool
}

type LiqPriceBin struct {
	Bps       int
	LongVol   float64
	ShortVol  float64
	TotalVol  float64
	Imbalance float64
}

type LiqWindow struct {
	LongVol      float64
	ShortVol     float64
	TotalVol     float64
	Imbalance    float64
	PriceBinsBps []int
	Bins         []LiqPriceBin
	SampleCount  int
	CoverageSec  int64
	Status       string
	Complete     bool
	Rel          LiqRelMetrics
}

type LiqSource struct {
	Source            string
	Coverage          string
	Status            string
	StreamConnected   bool
	CoverageSec       int64
	SampleCount       int
	LastEventAgeSec   int64
	Complete          bool
	LastReconnectTime int64
	LastEventTime     int64
	LastGapResetTime  int64
}

type MarketSnapshot struct {
	Timestamp            time.Time
	DataAgeSec           map[string]int64
	Klines               map[string]map[string][]Candle
	OI                   map[string]OIBlock
	Funding              map[string]FundingBlock
	LongShort            map[string]map[string]LSRBlock
	FearGreed            *FearGreedPoint
	Liquidations         map[string]LiqBlock
	LiquidationsByWindow map[string]map[string]LiqWindow
	LiquidationSource    map[string]LiqSource
}
