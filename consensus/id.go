package consensus

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/baid64"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/operation/commit.rs
// Upstream-File-SHA256: 54f78964b48a910216c5552cee446d296b7499327acdd45cfa6f5046c0f6cad4
// Translation-Revision: 1

const (
	OperationCommitmentTag  = "urn:lnp-bp:rgb:operation#2024-02-03"
	SecretSealCommitmentTag = "urn:lnp-bp:seals:secret#2024-02-03"
)

var ErrInvalidIDLength = errors.New("RGB11 ID must contain exactly 32 bytes")

type ID [32]byte
type ContractID ID
type OperationID ID
type SecretSeal ID

func IDFromBytes(data []byte) (ID, error) {
	if len(data) != 32 {
		return ID{}, ErrInvalidIDLength
	}
	var id ID
	copy(id[:], data)
	return id, nil
}

func IDFromHex(value string) (ID, error) {
	data, err := hex.DecodeString(value)
	if err != nil {
		return ID{}, fmt.Errorf("decode RGB11 ID: %w", err)
	}
	return IDFromBytes(data)
}

func (id ID) Bytes() [32]byte { return [32]byte(id) }
func (id ID) Hex() string     { return hex.EncodeToString(id[:]) }

func (id OperationID) Hex() string { return ID(id).Hex() }
func (id SecretSeal) Hex() string  { return ID(id).Hex() }

func ParseContractID(value string) (ContractID, error) {
	payload, err := baid64.Decode32(value, baid64.RGBContractOptions())
	if err != nil {
		return ContractID{}, err
	}
	return ContractID(payload), nil
}

func (id ContractID) String() string {
	value, err := baid64.Encode32([32]byte(id), baid64.RGBContractOptions())
	if err != nil {
		panic(err)
	}
	return value
}

func ParseSecretSeal(value string) (SecretSeal, error) {
	payload, err := baid64.Decode32(value, baid64.UTXOBlindOptions())
	if err != nil {
		return SecretSeal{}, err
	}
	return SecretSeal(payload), nil
}

func (id SecretSeal) String() string {
	value, err := baid64.Encode32([32]byte(id), baid64.UTXOBlindOptions())
	if err != nil {
		panic(err)
	}
	return value
}
