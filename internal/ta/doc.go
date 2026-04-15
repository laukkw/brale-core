// Package ta provides pure-Go technical analysis indicator implementations.
//
// All functions are stateless, accept raw OHLCV slices, and return computed
// indicator series of the same length as the input (NaN-padded where the
// lookback window is insufficient).
//
// This package replaces the former go-talib CGO dependency and serves as the
// single source of truth for indicator computation in the brale-core runtime.
package ta
