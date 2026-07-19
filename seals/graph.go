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
// Translation-Revision: 1

var ErrInvalidGraphSeal = errors.New("invalid RGB11 graph seal")

// GraphBlindSeal is the BlindSeal<TxPtr> used by transition assignments. A
// nil TxID represents TxPtr::WitnessTx and is the normal blind-invoice form.
type GraphBlindSeal struct {
	TxID     *[32]byte
	Vout     uint32
	Blinding uint64
}

func NewWitnessBlindSeal(vout uint32, blinding uint64) GraphBlindSeal {
	return GraphBlindSeal{Vout: vout, Blinding: blinding}
}

func RandomWitnessBlindSeal(vout uint32) (GraphBlindSeal, error) {
	var entropy [8]byte
	if _, err := io.ReadFull(rand.Reader, entropy[:]); err != nil {
		return GraphBlindSeal{}, fmt.Errorf("generate seal blinding: %w", err)
	}
	return NewWitnessBlindSeal(vout, binary.LittleEndian.Uint64(entropy[:])), nil
}

func NewGraphBlindSeal(txid []byte, vout uint32, blinding uint64) (GraphBlindSeal, error) {
	if len(txid) != 32 {
		return GraphBlindSeal{}, ErrInvalidGraphSeal
	}
	var id [32]byte
	copy(id[:], txid)
	return GraphBlindSeal{TxID: &id, Vout: vout, Blinding: blinding}, nil
}

func (s GraphBlindSeal) StrictEncode(w io.Writer) error {
	e := strict.NewEncoder(w)
	if s.TxID == nil {
		if err := e.U8(0); err != nil {
			return err
		}
	} else {
		if err := e.U8(1); err != nil {
			return err
		}
		if err := e.Raw(s.TxID[:]); err != nil {
			return err
		}
	}
	if err := e.U32(s.Vout); err != nil {
		return err
	}
	return e.U64(s.Blinding)
}

func (s GraphBlindSeal) StrictBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := s.StrictEncode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeGraphBlindSeal(data []byte) (GraphBlindSeal, error) {
	r := bytes.NewReader(data)
	var seal GraphBlindSeal
	switch len(data) {
	case 13:
		tag, err := r.ReadByte()
		if err != nil || tag != 0 {
			return GraphBlindSeal{}, ErrInvalidGraphSeal
		}
	case 45:
		tag, err := r.ReadByte()
		if err != nil || tag != 1 {
			return GraphBlindSeal{}, ErrInvalidGraphSeal
		}
		var txid [32]byte
		if _, err := io.ReadFull(r, txid[:]); err != nil {
			return GraphBlindSeal{}, ErrInvalidGraphSeal
		}
		seal.TxID = &txid
	default:
		return GraphBlindSeal{}, ErrInvalidGraphSeal
	}
	if err := binary.Read(r, binary.LittleEndian, &seal.Vout); err != nil {
		return GraphBlindSeal{}, ErrInvalidGraphSeal
	}
	if err := binary.Read(r, binary.LittleEndian, &seal.Blinding); err != nil || r.Len() != 0 {
		return GraphBlindSeal{}, ErrInvalidGraphSeal
	}
	return seal, nil
}

func (s GraphBlindSeal) Conceal() (consensus.SecretSeal, error) {
	encoded, err := s.StrictBytes()
	if err != nil {
		return consensus.SecretSeal{}, err
	}
	hash := consensus.TaggedHash(consensus.SecretSealCommitmentTag, encoded)
	return consensus.SecretSeal(hash), nil
}
