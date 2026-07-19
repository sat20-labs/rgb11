package operations

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/operation/commit.rs
// Upstream-File: src/operation/operations.rs
// Translation-Revision: 1

var ErrInvalidGenesis = errors.New("invalid decoded RGB11 genesis")

type GenesisCommitment struct {
	IssuerHash      [32]byte
	MetadataHash    [32]byte
	GlobalsRoot     [32]byte
	InputsRoot      [32]byte
	AssignmentsRoot [32]byte
	OperationID     [32]byte
}

func CommitGenesis(value strict_types.Value) (GenesisCommitment, error) {
	if value.Kind != strict_types.ValueStruct {
		return GenesisCommitment{}, ErrInvalidGenesis
	}
	ffv, err := requiredField(value, "ffv")
	if err != nil {
		return GenesisCommitment{}, err
	}
	schemaID, err := requiredField(value, "schemaId")
	if err != nil {
		return GenesisCommitment{}, err
	}
	timestamp, err := requiredField(value, "timestamp")
	if err != nil {
		return GenesisCommitment{}, err
	}
	issuer, err := requiredField(value, "issuer")
	if err != nil {
		return GenesisCommitment{}, err
	}
	chainNet, err := requiredField(value, "chainNet")
	if err != nil {
		return GenesisCommitment{}, err
	}
	strategy, err := requiredField(value, "sealClosingStrategy")
	if err != nil {
		return GenesisCommitment{}, err
	}
	metadata, err := requiredField(value, "metadata")
	if err != nil {
		return GenesisCommitment{}, err
	}
	globals, err := requiredField(value, "globals")
	if err != nil {
		return GenesisCommitment{}, err
	}
	assignments, err := requiredField(value, "assignments")
	if err != nil {
		return GenesisCommitment{}, err
	}

	commitment := GenesisCommitment{
		IssuerHash:   consensus.StrictValueHash(issuer.Encoded),
		MetadataHash: consensus.StrictValueHash(metadata.Encoded),
		InputsRoot:   consensus.MerkleRoot(nil),
	}
	commitment.GlobalsRoot, err = commitGlobals(globals)
	if err != nil {
		return GenesisCommitment{}, err
	}
	commitment.AssignmentsRoot, err = commitAssignments(assignments)
	if err != nil {
		return GenesisCommitment{}, err
	}

	encoded := make([]byte, 0, 1+8+1+32+8+32+2+32*4)
	encoded = append(encoded, ffv.Encoded...)
	var nonce [8]byte
	binary.LittleEndian.PutUint64(nonce[:], ^uint64(0))
	encoded = append(encoded, nonce[:]...)
	encoded = append(encoded, 0) // TypeCommitment::Genesis
	encoded = append(encoded, schemaID.Encoded...)
	encoded = append(encoded, timestamp.Encoded...)
	encoded = append(encoded, commitment.IssuerHash[:]...)
	encoded = append(encoded, chainNet.Encoded...)
	encoded = append(encoded, strategy.Encoded...)
	encoded = append(encoded, commitment.MetadataHash[:]...)
	encoded = append(encoded, commitment.GlobalsRoot[:]...)
	encoded = append(encoded, commitment.InputsRoot[:]...)
	encoded = append(encoded, commitment.AssignmentsRoot[:]...)
	commitment.OperationID = consensus.TaggedHash(consensus.OperationCommitmentTag, encoded)
	return commitment, nil
}

func GenesisContractID(value strict_types.Value) (string, error) {
	commitment, err := CommitGenesis(value)
	if err != nil {
		return "", err
	}
	return baid64.Encode32(commitment.OperationID, baid64.RGBContractOptions())
}

func commitGlobals(value strict_types.Value) ([32]byte, error) {
	entries, ok := asMap(value)
	if !ok {
		return [32]byte{}, fmt.Errorf("%w: globals", ErrInvalidGenesis)
	}
	leaves := make([][32]byte, 0)
	for _, entry := range entries {
		states, ok := asList(entry.Value)
		if !ok {
			return [32]byte{}, fmt.Errorf("%w: global state", ErrInvalidGenesis)
		}
		for _, state := range states {
			encoded := append(append([]byte(nil), entry.Key.Encoded...), state.Encoded...)
			leaves = append(leaves, consensus.MerkleLeafHash(encoded))
		}
	}
	return consensus.MerkleRoot(leaves), nil
}

func commitAssignments(value strict_types.Value) ([32]byte, error) {
	entries, ok := asMap(value)
	if !ok {
		return [32]byte{}, fmt.Errorf("%w: assignments", ErrInvalidGenesis)
	}
	leaves := make([][32]byte, 0)
	for _, entry := range entries {
		typed := unwrap(entry.Value)
		if typed.Kind != strict_types.ValueUnion || typed.Inner == nil {
			return [32]byte{}, fmt.Errorf("%w: typed assignments", ErrInvalidGenesis)
		}
		assigns, ok := asList(*typed.Inner)
		if !ok {
			return [32]byte{}, fmt.Errorf("%w: assignment list", ErrInvalidGenesis)
		}
		for _, assignment := range assigns {
			assignment = unwrap(assignment)
			if assignment.Kind != strict_types.ValueUnion || assignment.Inner == nil {
				return [32]byte{}, fmt.Errorf("%w: assignment", ErrInvalidGenesis)
			}
			fields := unwrap(*assignment.Inner)
			sealValue, err := requiredField(fields, "seal")
			if err != nil {
				return [32]byte{}, err
			}
			state, err := requiredField(fields, "state")
			if err != nil {
				return [32]byte{}, err
			}
			secret, err := assignmentSecret(assignment.Name, sealValue)
			if err != nil {
				return [32]byte{}, err
			}
			encoded := append(append([]byte(nil), entry.Key.Encoded...), state.Encoded...)
			encoded = append(encoded, secret[:]...)
			leaves = append(leaves, consensus.MerkleLeafHash(encoded))
		}
	}
	return consensus.MerkleRoot(leaves), nil
}

func assignmentSecret(kind string, value strict_types.Value) ([32]byte, error) {
	if kind == "confidentialSeal" {
		raw, ok := asBytes(value)
		if !ok || len(raw) != 32 {
			return [32]byte{}, fmt.Errorf("%w: confidential seal", ErrInvalidGenesis)
		}
		var secret [32]byte
		copy(secret[:], raw)
		return secret, nil
	}
	if kind != "revealed" {
		return [32]byte{}, fmt.Errorf("%w: assignment kind %s", ErrInvalidGenesis, kind)
	}
	value = unwrap(value)
	txidValue, err := requiredField(value, "txid")
	if err != nil {
		return [32]byte{}, err
	}
	voutValue, err := requiredField(value, "vout")
	if err != nil {
		return [32]byte{}, err
	}
	vout, ok := asUint(voutValue)
	if !ok || vout > ^uint64(uint32(0)) {
		return [32]byte{}, fmt.Errorf("%w: seal vout", ErrInvalidGenesis)
	}
	blindingValue, err := requiredField(value, "blinding")
	if err != nil {
		return [32]byte{}, err
	}
	blinding, ok := asUint(blindingValue)
	if !ok {
		return [32]byte{}, fmt.Errorf("%w: seal blinding", ErrInvalidGenesis)
	}
	if txid, ok := asBytes(txidValue); ok {
		if len(txid) != 32 {
			return [32]byte{}, fmt.Errorf("%w: seal txid", ErrInvalidGenesis)
		}
		seal, err := seals.NewBlindSeal(txid, uint32(vout), blinding)
		if err != nil {
			return [32]byte{}, err
		}
		secret, err := seal.Conceal()
		return [32]byte(secret), err
	}
	txPointer := unwrap(txidValue)
	if txPointer.Kind != strict_types.ValueUnion {
		return [32]byte{}, fmt.Errorf("%w: graph seal tx pointer", ErrInvalidGenesis)
	}
	var graph seals.GraphBlindSeal
	switch txPointer.Name {
	case "witnessTx":
		graph = seals.NewWitnessBlindSeal(uint32(vout), blinding)
	case "txid":
		if txPointer.Inner == nil {
			return [32]byte{}, fmt.Errorf("%w: graph seal txid", ErrInvalidGenesis)
		}
		txid, ok := asBytes(*txPointer.Inner)
		if !ok || len(txid) != 32 {
			return [32]byte{}, fmt.Errorf("%w: graph seal txid", ErrInvalidGenesis)
		}
		var err error
		graph, err = seals.NewGraphBlindSeal(txid, uint32(vout), blinding)
		if err != nil {
			return [32]byte{}, err
		}
	default:
		return [32]byte{}, fmt.Errorf("%w: graph seal tx pointer %s", ErrInvalidGenesis, txPointer.Name)
	}
	secret, err := graph.Conceal()
	return [32]byte(secret), err
}

func requiredField(value strict_types.Value, name string) (strict_types.Value, error) {
	value = unwrap(value)
	field, ok := value.Field(name)
	if !ok {
		return strict_types.Value{}, fmt.Errorf("%w: missing %s", ErrInvalidGenesis, name)
	}
	return field, nil
}

func unwrap(value strict_types.Value) strict_types.Value {
	for value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		value = value.Items[0]
	}
	return value
}

func asMap(value strict_types.Value) ([]strict_types.Entry, bool) {
	value = unwrap(value)
	return value.Entries, value.Kind == strict_types.ValueMap
}

func asList(value strict_types.Value) ([]strict_types.Value, bool) {
	value = unwrap(value)
	if value.Kind != strict_types.ValueList && value.Kind != strict_types.ValueSet {
		return nil, false
	}
	return value.Items, true
}

func asBytes(value strict_types.Value) ([]byte, bool) {
	value = unwrap(value)
	return value.Raw, value.Kind == strict_types.ValueBytes
}

func asUint(value strict_types.Value) (uint64, bool) {
	value = unwrap(value)
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return asUint(*value.Inner)
	}
	if value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		return asUint(value.Items[0])
	}
	return value.Uint64()
}
