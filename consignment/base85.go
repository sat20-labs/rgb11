package consignment

import (
	"errors"
	"fmt"
)

// Upstream-Repository: https://github.com/darkwyrm/base85
// Upstream-Version: 2.0.0
// Upstream-File: src/lib.rs
// Translation-Revision: 1

const base85Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"

var (
	ErrBase85Character = errors.New("invalid RFC1924 base85 character")
	ErrBase85Remainder = errors.New("unexpected RFC1924 base85 remainder")
)

func encodeBase85(data []byte) string {
	encodedLen := len(data) / 4 * 5
	if remainder := len(data) % 4; remainder != 0 {
		encodedLen += remainder + 1
	}
	out := make([]byte, 0, encodedLen)
	for offset := 0; offset < len(data); offset += 4 {
		remaining := len(data) - offset
		width := min(4, remaining)
		var block [4]byte
		copy(block[:], data[offset:offset+width])
		value := uint32(block[0])<<24 | uint32(block[1])<<16 | uint32(block[2])<<8 | uint32(block[3])
		var digits [5]byte
		for index := len(digits) - 1; index >= 0; index-- {
			digits[index] = base85Alphabet[value%85]
			value /= 85
		}
		encodedWidth := 5
		if width < 4 {
			encodedWidth = width + 1
		}
		out = append(out, digits[:encodedWidth]...)
	}
	return string(out)
}

func decodeBase85(value string) ([]byte, error) {
	if len(value)%5 == 1 {
		return nil, ErrBase85Remainder
	}
	decode := func(char byte) (uint32, error) {
		for index := 0; index < len(base85Alphabet); index++ {
			if base85Alphabet[index] == char {
				return uint32(index), nil
			}
		}
		return 0, fmt.Errorf("%w: %q", ErrBase85Character, char)
	}
	out := make([]byte, 0, len(value)/5*4+4)
	for offset := 0; offset < len(value); offset += 5 {
		remaining := len(value) - offset
		width := min(5, remaining)
		if width < 2 {
			return nil, ErrBase85Remainder
		}
		var digits [5]uint32
		for index := range digits {
			if index < width {
				var err error
				digits[index], err = decode(value[offset+index])
				if err != nil {
					return nil, err
				}
			} else {
				// The upstream decoder pads a partial group with '~' (84).
				digits[index] = 84
			}
		}
		accumulator := digits[0]*85*85*85*85 + digits[1]*85*85*85 +
			digits[2]*85*85 + digits[3]*85 + digits[4]
		bytes := [4]byte{byte(accumulator >> 24), byte(accumulator >> 16), byte(accumulator >> 8), byte(accumulator)}
		out = append(out, bytes[:width-1]...)
	}
	return out, nil
}
