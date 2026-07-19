// Package sync defines the compact payload stored in a wallet-owned RGB11
// DKVS head record. Authentication belongs to the enclosing DKVS record: the
// wallet signs that record and DKVS selects its latest active sequence. Keeping
// another signature inside the payload would authenticate the same bytes twice
// without adding a second trust boundary.
package sync

import (
	"bytes"
	"crypto/sha256"
	"errors"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

const HeadVersion uint32 = 1

var (
	ErrHeadWallet   = errors.New("RGB11 head belongs to another wallet")
	ErrHeadSequence = errors.New("invalid RGB11 wallet head sequence")
	ErrHeadConflict = errors.New("conflicting RGB11 wallet heads at the same sequence")
	ErrHeadField    = errors.New("invalid RGB11 wallet head field")
)

// WalletHead contains only state identity and ordering data. The enclosing
// DKVS record supplies owner pubkey, record sequence, signature and TTL.
type WalletHead struct {
	Version     uint32   `json:"version"`
	WalletID    string   `json:"wallet_id"`
	Seq         uint64   `json:"seq"`
	StateHash   [32]byte `json:"state_hash"`
	OperationID [32]byte `json:"operation_id"`
}

func (h WalletHead) Validate(walletID string) error {
	if h.Version != HeadVersion || h.WalletID == "" || len(h.WalletID) > 128 || h.Seq == 0 ||
		h.StateHash == ([32]byte{}) || h.OperationID == ([32]byte{}) {
		return ErrHeadField
	}
	if walletID != "" && h.WalletID != walletID {
		return ErrHeadWallet
	}
	return nil
}

// StrictEncode is the canonical payload covered by the outer DKVS record
// signature and used as the stable identity of this latest-head value.
func (h WalletHead) StrictEncode() ([]byte, error) {
	if err := h.Validate(""); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	e := strict.NewEncoder(&buf)
	if err := e.U32(h.Version); err != nil {
		return nil, err
	}
	if err := e.String(h.WalletID, 1, 128); err != nil {
		return nil, err
	}
	if err := e.U64(h.Seq); err != nil {
		return nil, err
	}
	if err := e.Raw(h.StateHash[:]); err != nil {
		return nil, err
	}
	if err := e.Raw(h.OperationID[:]); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h WalletHead) Hash() ([32]byte, error) {
	encoded, err := h.StrictEncode()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func (h WalletHead) ValidateSuccessor(previous *WalletHead) error {
	if previous == nil {
		if h.Seq != 1 {
			return ErrHeadSequence
		}
		return nil
	}
	if h.WalletID != previous.WalletID {
		return ErrHeadWallet
	}
	if h.Seq != previous.Seq+1 {
		return ErrHeadSequence
	}
	return nil
}

// SelectLatest selects the unique highest sequence from payloads whose outer
// DKVS records have already passed signature/owner/selector verification.
func SelectLatest(candidates []WalletHead, walletID string) (*WalletHead, error) {
	var latest *WalletHead
	for index := range candidates {
		candidate := &candidates[index]
		if err := candidate.Validate(walletID); err != nil {
			return nil, err
		}
		if latest == nil || candidate.Seq > latest.Seq {
			copy := *candidate
			latest = &copy
			continue
		}
		if candidate.Seq == latest.Seq {
			left, err := candidate.Hash()
			if err != nil {
				return nil, err
			}
			right, err := latest.Hash()
			if err != nil {
				return nil, err
			}
			if left != right {
				return nil, ErrHeadConflict
			}
		}
	}
	return latest, nil
}
