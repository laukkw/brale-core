package ta

// OBV computes On-Balance Volume. OBV[0] = 0; for i > 0, OBV accumulates
// volume when close rises and subtracts when close falls.
func OBV(closes, volumes []float64) ([]float64, error) {
	if err := ValidatePairSeries("closes", closes, "volumes", volumes); err != nil {
		return nil, err
	}
	out := make([]float64, len(closes))
	for i := 1; i < len(closes); i++ {
		switch {
		case closes[i] > closes[i-1]:
			out[i] = out[i-1] + volumes[i]
		case closes[i] < closes[i-1]:
			out[i] = out[i-1] - volumes[i]
		default:
			out[i] = out[i-1]
		}
	}
	return out, nil
}
