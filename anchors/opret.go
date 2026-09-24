package anchors

import (
	"bytes"
	"errors"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1
// Upstream-Commit: 32a67862aef0f4c7a1fdc7834a3555d702f1bdf0
// Upstream-File: src/dbc/opret/spk.rs
// Translation-Revision: 1

var ErrInvalidOpret = errors.New("invalid RGB11 opret commitment")

func OpretScript(commitment [32]byte) []byte {
	script := make([]byte, 34)
	script[0], script[1] = 0x6a, 0x20
	copy(script[2:], commitment[:])
	return script
}

func VerifyOpretScript(script []byte, commitment [32]byte) error {
	if len(script) != 34 || script[0] != 0x6a || script[1] != 0x20 || !bytes.Equal(script[2:], commitment[:]) {
		return ErrInvalidOpret
	}
	return nil
}
