package direction

import "testing"

func TestComputeConsensusAddsResonanceForAlignedTrend(t *testing.T) {
	consensus := ComputeConsensusWithThresholds(
		Evidence{Source: SourceIndicator, Score: -0.62, Confidence: 0.55},
		Evidence{Source: SourceStructure, Score: -0.58, Confidence: 0.58},
		Evidence{Source: SourceMechanics, Score: -0.10, Confidence: 0.05},
		0.35,
		0.52,
	)

	if !consensus.Resonance.Active {
		t.Fatalf("resonance inactive")
	}
	if consensus.Resonance.Bonus <= 0 {
		t.Fatalf("resonance bonus=%v want > 0", consensus.Resonance.Bonus)
	}
	if consensus.Confidence < 0.52 {
		t.Fatalf("confidence=%v want >= 0.52", consensus.Confidence)
	}
	if consensus.Direction != "short" {
		t.Fatalf("direction=%s want short", consensus.Direction)
	}
}

func TestComputeConsensusSuppressesResonanceOnStrongOpposition(t *testing.T) {
	consensus := ComputeConsensusWithThresholds(
		Evidence{Source: SourceIndicator, Score: -0.62, Confidence: 0.55},
		Evidence{Source: SourceStructure, Score: -0.58, Confidence: 0.58},
		Evidence{Source: SourceMechanics, Score: 0.65, Confidence: 0.72},
		0.35,
		0.52,
	)

	if consensus.Resonance.Active {
		t.Fatalf("resonance active with strong opposing evidence")
	}
	if consensus.Resonance.Bonus != 0 {
		t.Fatalf("resonance bonus=%v want 0", consensus.Resonance.Bonus)
	}
}
