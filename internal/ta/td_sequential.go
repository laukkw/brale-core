package ta

// TDSequential computes the Tom DeMark Sequential buy/sell setup counts.
// Buy setup: consecutive bars where close < close[i-4].
// Sell setup: consecutive bars where close > close[i-4].
func TDSequential(closes []float64) (buySetup, sellSetup []int) {
	n := len(closes)
	buySetup = make([]int, n)
	sellSetup = make([]int, n)
	if n <= 4 {
		return
	}
	for i := 4; i < n; i++ {
		if closes[i] < closes[i-4] {
			buySetup[i] = buySetup[i-1] + 1
			sellSetup[i] = 0
		} else if closes[i] > closes[i-4] {
			sellSetup[i] = sellSetup[i-1] + 1
			buySetup[i] = 0
		}
	}
	return
}
