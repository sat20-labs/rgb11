// Package assetid bridges the RGB ContractId string form into SAT20's
// AssetName ticker without changing the RGB consensus identifier.
package assetid

import (
	"errors"
	"strings"
)

const Prefix = "rgb:"

var (
	ErrEmpty         = errors.New("empty RGB11 asset ID")
	ErrMissingPrefix = errors.New("RGB11 asset ID must start with rgb:")
	ErrInvalidTicker = errors.New("invalid RGB11 ticker")
)

// Ticker removes exactly one leading "rgb:" prefix. The remaining official
// identifier is stored verbatim and is not normalized or encoded.
func Ticker(assetID string) (string, error) {
	if assetID == "" {
		return "", ErrEmpty
	}
	if !strings.HasPrefix(assetID, Prefix) {
		return "", ErrMissingPrefix
	}
	ticker := strings.TrimPrefix(assetID, Prefix)
	if ticker == "" || strings.Contains(ticker, ":") {
		return "", ErrInvalidTicker
	}
	return ticker, nil
}

// AssetID reconstructs the official identifier from a SAT20 ticker.
func AssetID(ticker string) (string, error) {
	if ticker == "" || strings.Contains(ticker, ":") {
		return "", ErrInvalidTicker
	}
	return Prefix + ticker, nil
}
