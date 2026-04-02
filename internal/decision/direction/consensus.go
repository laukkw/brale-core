package direction

import "math"

type Source string

const (
	SourceIndicator Source = "indicator"
	SourceStructure Source = "structure"
	SourceMechanics Source = "mechanics"
)

type Evidence struct {
	Source     Source
	Score      float64
	Confidence float64
}

type Consensus struct {
	Score      float64
	Confidence float64
	Agreement  float64
	Direction  string
	Resonance  Resonance
}

type Resonance struct {
	Active bool
	Bonus  float64
}

const (
	weightStructure = 1.0
	weightIndicator = 0.7
	weightMechanics = 0.5
	confidencePower = 1.0

	thresholdScore = 0.35
	thresholdConf  = 0.52

	resonanceMinConfidence    = 0.35
	resonanceMinScoreAbs      = 0.30
	resonanceOpposeConfidence = 0.55
	resonanceOpposeScoreAbs   = 0.45
	resonanceBonusScale       = 0.4
	resonanceBonusCap         = 0.12
)

func ThresholdScore() float64 {
	return thresholdScore
}

func ThresholdConfidence() float64 {
	return thresholdConf
}

func IsConsensusPassed(score, confidence float64) bool {
	return IsConsensusPassedWithThresholds(score, confidence, thresholdScore, thresholdConf)
}

func IsConsensusPassedWithThresholds(score, confidence, scoreThreshold, confidenceThreshold float64) bool {
	return math.Abs(score) >= scoreThreshold && confidence >= confidenceThreshold
}

/*
计算方法。
Let:
  i  = Source index {Structure, Indicator, Mechanics}
  Bi = Base Weight for source i (1.0, 0.7, 0.5)
  Si = Score from source i [-1, 1]
  Ci = Confidence from source i [0, 1]
1. Effective Weight (Wi):
   Wi = Bi * Ci
   (Lower curvature keeps early trend evidence in play instead of crushing medium-confidence reads.)
2. Aggregations:
   SumW     = Σ Wi
   SumWS    = Σ (Wi * Si)
   SumWSign = Σ (Wi * sgn(Si))
   SumBase  = Σ Bi
3. Derived Metrics:
   Score      = SumWS / SumW
   Agreement  = |SumWSign| / SumW   (0 = Total conflict, 1 = Total consensus)
   Coverage   = SumW / SumBase      (Ratio of "active" weight vs "total potential" weight)
   Confidence = Coverage * Agreement
4. Direction Decision:
   IF |Score| >= scoreThreshold
      AND Confidence >= confidenceThreshold
   THEN
      Direction = sgn(Score)
   ELSE
      Direction = "none"
*/

func ComputeConsensus(ind Evidence, st Evidence, mech Evidence) Consensus {
	return ComputeConsensusWithThresholds(ind, st, mech, thresholdScore, thresholdConf)
}

func ComputeConsensusWithThresholds(ind Evidence, st Evidence, mech Evidence, scoreThreshold, confidenceThreshold float64) Consensus {
	items := []Evidence{ind, st, mech}
	base := map[Source]float64{
		SourceStructure: weightStructure,
		SourceIndicator: weightIndicator,
		SourceMechanics: weightMechanics,
	}

	var sumW float64
	var sumWS float64
	var sumBase float64
	var sumWSign float64
	validItems := make([]weightedEvidence, 0, len(items))

	for _, item := range items {
		bw := base[item.Source]
		sumBase += bw
		score, okScore := sanitizeScore(item.Score)
		conf, okConf := sanitizeConf(item.Confidence)
		if !okScore || !okConf {
			continue
		}
		w := bw * math.Pow(conf, confidencePower)
		if w <= 0 {
			continue
		}
		sumW += w
		sumWS += w * score
		sumWSign += w * sign(score)
		validItems = append(validItems, weightedEvidence{
			Source:     item.Source,
			BaseWeight: bw,
			Score:      score,
			Confidence: conf,
		})
	}

	if sumW <= 0 || sumBase <= 0 {
		return Consensus{Score: 0, Confidence: 0, Agreement: 0, Direction: "none"}
	}

	score := clamp(sumWS/sumW, -1, 1)
	agreement := clamp(math.Abs(sumWSign)/sumW, 0, 1)
	coverage := clamp(sumW/sumBase, 0, 1)
	baseConfidence := clamp(coverage*agreement, 0, 1)
	resonance := computeResonance(validItems, sumBase, score)
	confidence := clamp(baseConfidence+resonance.Bonus, 0, 1)

	direction := "none"
	if IsConsensusPassedWithThresholds(score, confidence, scoreThreshold, confidenceThreshold) {
		if score > 0 {
			direction = "long"
		} else if score < 0 {
			direction = "short"
		}
	}

	return Consensus{
		Score:      score,
		Confidence: confidence,
		Agreement:  agreement,
		Direction:  direction,
		Resonance:  resonance,
	}
}

type weightedEvidence struct {
	Source     Source
	BaseWeight float64
	Score      float64
	Confidence float64
}

func computeResonance(items []weightedEvidence, sumBase, consensusScore float64) Resonance {
	if len(items) < 2 || sumBase <= 0 {
		return Resonance{}
	}
	dominantSign := sign(consensusScore)
	if dominantSign == 0 {
		return Resonance{}
	}

	aligned := make([]weightedEvidence, 0, len(items))
	var alignedBaseWeight float64
	var alignedConfidence float64
	var alignedScoreAbs float64

	for _, item := range items {
		scoreSign := sign(item.Score)
		if scoreSign == dominantSign {
			if item.Confidence < resonanceMinConfidence || math.Abs(item.Score) < resonanceMinScoreAbs {
				continue
			}
			aligned = append(aligned, item)
			alignedBaseWeight += item.BaseWeight
			alignedConfidence += item.Confidence
			alignedScoreAbs += math.Abs(item.Score)
			continue
		}
		if scoreSign == -dominantSign && item.Confidence >= resonanceOpposeConfidence && math.Abs(item.Score) >= resonanceOpposeScoreAbs {
			return Resonance{}
		}
	}

	if len(aligned) < 2 {
		return Resonance{}
	}

	alignedWeightRatio := clamp(alignedBaseWeight/sumBase, 0, 1)
	alignedConfAvg := clamp(alignedConfidence/float64(len(aligned)), 0, 1)
	alignedScoreAvg := clamp(alignedScoreAbs/float64(len(aligned)), 0, 1)
	strength := alignedWeightRatio * alignedConfAvg * alignedScoreAvg
	bonus := math.Min(resonanceBonusCap, resonanceBonusScale*strength)
	if bonus <= 0 {
		return Resonance{}
	}
	return Resonance{
		Active: true,
		Bonus:  bonus,
	}
}

func sanitizeScore(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value < -1 || value > 1 {
		return 0, false
	}
	return value, true
}

func sanitizeConf(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value < 0 || value > 1 {
		return 0, false
	}
	return value, true
}

func sign(value float64) float64 {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
