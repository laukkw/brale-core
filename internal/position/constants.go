package position

// R-multiple bucket boundaries for stop-loss-relative classification in
// BuildPositionSummary. The value "relative" is (SL − entry) / |risk|,
// so ≤ RBucketNeg1 means the stop is more than 0.75 R below entry, etc.
const (
	RBucketNeg1    = -0.75 // below → "-1R"
	RBucketNegHalf = -0.25 // below → "-0.5R"
	RBucketPosHalf = 0.25  // below → "BE"
	RBucketPos1    = 0.75  // below → "+0.5R", above → "+1R"
)

// DustThresholdRatio is the minimum fraction of a position considered
// negligible ("dust") when deciding whether to close the entire
// remaining position. 0.1% of position size.
const DustThresholdRatio = 0.001
