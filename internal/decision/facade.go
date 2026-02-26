package decision

import (
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
)

type AgentRunner = agent.Runner
type ProviderRunner = provider.Runner

type IndicatorSummary = agent.IndicatorSummary
type StructureSummary = agent.StructureSummary
type MechanicsSummary = agent.MechanicsSummary

type ProviderBundle = fund.ProviderBundle
type GateDecision = fund.GateDecision

type FeatureCompressor = features.Compressor
type DefaultIndicatorBuilder = features.DefaultIndicatorBuilder
type IntervalTrendBuilder = features.IntervalTrendBuilder
type DefaultMechanicsBuilder = features.DefaultMechanicsBuilder
type TrendCompressOptions = features.TrendCompressOptions
type MechanicsCompressOptions = features.MechanicsCompressOptions
type IndicatorCompressOptions = features.IndicatorCompressOptions

type Formatter = decisionfmt.Formatter

func NewFormatter() decisionfmt.Formatter {
	return decisionfmt.New()
}
