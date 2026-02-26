package numutil

import "math"

func AbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func AbsFloat(v float64) float64 {
	return math.Abs(v)
}

func MaxFloat(a, b float64) float64 {
	if a >= b {
		return a
	}
	return b
}

func MinFloat(a, b float64) float64 {
	if a <= b {
		return a
	}
	return b
}
