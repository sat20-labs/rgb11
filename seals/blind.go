package seals

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/sat20-labs/rgb11/consensus"
	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/seals/txout/blind.rs
// Upstream-File-SHA256: 82eb02354b6e4ee841667401b74e564d417214f63aaac58d33ecebe59d79b899
// Translation-Revision: 1

var ErrInvalidTxID = errors.New("transaction ID must contain 32 bytes")

// BlindSeal is a revealed single-use seal. TxID uses Bitcoin wire byte order,
// matching the strict encoding of bitcoin::Txid in the frozen Rust source.
type BlindSeal struct {
	TxID     [32]byte
	Vout     uint32
	Blinding uint64
}

func NewBlindSeal(txid []byte, vout uint32, blinding uint64) (BlindSeal, error) {
	if len(txid) != 32 {
		return BlindSeal{}, ErrInvalidTxID
	}
	var seal BlindSeal
	copy(seal.TxID[:], txid)
	seal.Vout = vout
	seal.Blinding = blinding
	return seal, nil
}

func RandomBlindSeal(txid []byte, vout uint32) (BlindSeal, error) {
	var entropy [8]byte
	if _, err := io.ReadFull(rand.Reader, entropy[:]); err != nil {
		return BlindSeal{}, fmt.Errorf("generate seal blinding: %w", err)
	}
	return NewBlindSeal(txid, vout, binary.LittleEndian.Uint64(entropy[:]))
}

func (s BlindSeal) StrictEncode(w io.Writer) error {
	e := strict.NewEncoder(w)
	if err := e.Raw(s.TxID[:]); err != nil {
		return err
	}
	if err := e.U32(s.Vout); err != nil {
		return err
	}
	return e.U64(s.Blinding)
}

func (s BlindSeal) StrictBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := s.StrictEncode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s BlindSeal) Conceal() (consensus.SecretSeal, error) {
	encoded, err := s.StrictBytes()
	if err != nil {
		return consensus.SecretSeal{}, err
	}
	hash := consensus.TaggedHash(consensus.SecretSealCommitmentTag, encoded)
	return consensus.SecretSeal(hash), nil
}
