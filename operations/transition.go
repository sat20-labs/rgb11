package operations

import (
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/operation/commit.rs
// Upstream-File: src/operation/operations.rs
// Translation-Revision: 1

var ErrInvalidTransition = errors.New("invalid decoded RGB11 transition")

type TransitionCommitment struct {
	MetadataHash    [32]byte
	GlobalsRoot     [32]byte
	InputsRoot      [32]byte
	AssignmentsRoot [32]byte
	OperationID     [32]byte
}

func CommitTransition(value strict_types.Value) (TransitionCommitment, error) {
	if value.Kind != strict_types.ValueStruct {
		return TransitionCommitment{}, ErrInvalidTransition
	}
	ffv, err := transitionField(value, "ffv")
	if err != nil {
		return TransitionCommitment{}, err
	}
	contractID, err := transitionField(value, "contractId")
	if err != nil {
		return TransitionCommitment{}, err
	}
	nonce, err := transitionField(value, "nonce")
	if err != nil {
		return TransitionCommitment{}, err
	}
	transitionType, err := transitionField(value, "transitionType")
	if err != nil {
		return TransitionCommitment{}, err
	}
	metadata, err := transitionField(value, "metadata")
	if err != nil {
		return TransitionCommitment{}, err
	}
	globals, err := transitionField(value, "globals")
	if err != nil {
		return TransitionCommitment{}, err
	}
	inputs, err := transitionField(value, "inputs")
	if err != nil {
		return TransitionCommitment{}, err
	}
	assignments, err := transitionField(value, "assignments")
	if err != nil {
		return TransitionCommitment{}, err
	}

	commitment := TransitionCommitment{MetadataHash: consensus.StrictValueHash(metadata.Encoded)}
	commitment.GlobalsRoot, err = commitGlobals(globals)
	if err != nil {
		return TransitionCommitment{}, fmt.Errorf("%w: globals: %v", ErrInvalidTransition, err)
	}
	commitment.InputsRoot, err = commitInputs(inputs)
	if err != nil {
		return TransitionCommitment{}, err
	}
	commitment.AssignmentsRoot, err = commitAssignments(assignments)
	if err != nil {
		return TransitionCommitment{}, fmt.Errorf("%w: assignments: %v", ErrInvalidTransition, err)
	}

	encoded := make([]byte, 0, len(ffv.Encoded)+len(nonce.Encoded)+1+len(contractID.Encoded)+len(transitionType.Encoded)+32*4)
	encoded = append(encoded, ffv.Encoded...)
	encoded = append(encoded, nonce.Encoded...)
	encoded = append(encoded, 1) // TypeCommitment::Transition
	encoded = append(encoded, contractID.Encoded...)
	encoded = append(encoded, transitionType.Encoded...)
	encoded = append(encoded, commitment.MetadataHash[:]...)
	encoded = append(encoded, commitment.GlobalsRoot[:]...)
	encoded = append(encoded, commitment.InputsRoot[:]...)
	encoded = append(encoded, commitment.AssignmentsRoot[:]...)
	commitment.OperationID = consensus.TaggedHash(consensus.OperationCommitmentTag, encoded)
	return commitment, nil
}

func commitInputs(value strict_types.Value) ([32]byte, error) {
	inputs, ok := asList(value)
	if !ok || len(inputs) == 0 {
		return [32]byte{}, fmt.Errorf("%w: inputs", ErrInvalidTransition)
	}
	leaves := make([][32]byte, 0, len(inputs))
	for _, input := range inputs {
		if len(input.Encoded) == 0 {
			return [32]byte{}, fmt.Errorf("%w: input encoding", ErrInvalidTransition)
		}
		leaves = append(leaves, consensus.MerkleLeafHash(input.Encoded))
	}
	return consensus.MerkleRoot(leaves), nil
}

func transitionField(value strict_types.Value, name string) (strict_types.Value, error) {
	field, ok := value.Field(name)
	if !ok {
		return strict_types.Value{}, fmt.Errorf("%w: missing %s", ErrInvalidTransition, name)
	}
	return field, nil
}
