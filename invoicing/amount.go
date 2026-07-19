// Package invoicing implements RGB 0.11.1 invoice parsing and formatting.
package invoicing

import (
	"encoding/base32"
	"encoding/binary"
	"errors"
)

// Upstream-Repository: rgb-protocol/rgb-ops
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 5308b9d46c91857513ff5be2459992264687632b
// Upstream-File: invoice/src/amount.rs
// Translation-Revision: 1

const amountAlphabet = "abcdefghkmnABCDEFGHKMNPQRSTVWXYZ"

var (
	amountEncoding   = base32.NewEncoding(amountAlphabet).WithPadding(base32.NoPadding)
	ErrInvalidAmount = errors.New("invalid RGB11 invoice amount")
)

type Amount uint64

func (a Amount) String() string {
	var le [8]byte
	binary.LittleEndian.PutUint64(le[:], uint64(a))
	length := 1
	for index := len(le) - 1; index >= 0; index-- {
		if le[index] != 0 {
			length = index + 1
			break
		}
	}
	return amountEncoding.EncodeToString(le[:length])
}

func ParseAmount(value string) (Amount, error) {
	if value == "" {
		return 0, ErrInvalidAmount
	}
	decoded, err := amountEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 || len(decoded) > 8 {
		return 0, ErrInvalidAmount
	}
	var le [8]byte
	copy(le[:], decoded)
	return Amount(binary.LittleEndian.Uint64(le[:])), nil
}
