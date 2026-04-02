package decision

import (
	"math"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/direction"
	"brale-core/internal/decision/fund"
)

func computeDirectionConsensus(enabled AgentEnabled, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, scoreThreshold, confThreshold float64) direction.Consensus {
	indConf := ind.MovementConfidence
	stConf := st.MovementConfidence
	mechConf := mech.MovementConfidence
	if !enabled.Indicator {
		indConf = 0
	}
	if !enabled.Structure {
		stConf = 0
	}
	if !enabled.Mechanics {
		mechConf = 0
	}
	return direction.ComputeConsensusWithThresholds(
		direction.Evidence{Source: direction.SourceIndicator, Score: ind.MovementScore, Confidence: indConf},
		direction.Evidence{Source: direction.SourceStructure, Score: st.MovementScore, Confidence: stConf},
		direction.Evidence{Source: direction.SourceMechanics, Score: mech.MovementScore, Confidence: mechConf},
		scoreThreshold,
		confThreshold,
	)
}

func gateError(reason string) fund.GateDecision {
	r := strings.TrimSpace(reason)
	if r == "" {
		r = "GATE_ERROR"
	}
	return fund.GateDecision{GlobalTradeable: false, DecisionAction: "VETO", GateReason: r, Direction: "none", Grade: 0}
}

func buildDirectionConsensusDerived(enabled AgentEnabled, res SymbolResult, scoreThreshold, confThreshold float64) map[string]any {
	return map[string]any{
		"score":                res.ConsensusScore,
		"confidence":           res.ConsensusConfidence,
		"agreement":            res.ConsensusAgreement,
		"direction":            res.ConsensusDirection,
		"resonance_bonus":      res.ConsensusResonance,
		"resonance_active":     res.ConsensusResonant,
		"score_threshold":      scoreThreshold,
		"confidence_threshold": confThreshold,
		"score_passed":         math.Abs(res.ConsensusScore) >= scoreThreshold,
		"confidence_passed":    res.ConsensusConfidence >= confThreshold,
		"passed":               direction.IsConsensusPassedWithThresholds(res.ConsensusScore, res.ConsensusConfidence, scoreThreshold, confThreshold),
		"sources": map[string]any{
			"indicator": buildDirectionConsensusSource(enabled.Indicator, res.AgentIndicator.MovementScore, res.AgentIndicator.MovementConfidence),
			"structure": buildDirectionConsensusSource(enabled.Structure, res.AgentStructure.MovementScore, res.AgentStructure.MovementConfidence),
			"mechanics": buildDirectionConsensusSource(enabled.Mechanics, res.AgentMechanics.MovementScore, res.AgentMechanics.MovementConfidence),
		},
	}
}

func resolveConsensusThresholds(cfg config.SymbolConfig) (float64, float64) {
	scoreThreshold := cfg.Consensus.ScoreThreshold
	confThreshold := cfg.Consensus.ConfidenceThreshold
	if scoreThreshold <= 0 {
		scoreThreshold = direction.ThresholdScore()
	}
	if confThreshold <= 0 {
		confThreshold = direction.ThresholdConfidence()
	}
	return scoreThreshold, confThreshold
}

func buildDirectionConsensusSource(enabled bool, score, confidence float64) map[string]any {
	usedConfidence := confidence
	if !enabled {
		usedConfidence = 0
	}
	return map[string]any{"enabled": enabled, "score": score, "confidence": usedConfidence, "raw_confidence": confidence}
}
