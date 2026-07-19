package operations

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/operation/bundle.rs
// Translation-Revision: 1

const BundleCommitmentTag = "urn:lnp-bp:rgb:bundle#2024-02-03"

var (
	ErrInvalidBundle        = errors.New("invalid decoded RGB11 transition bundle")
	ErrTransitionIDMismatch = errors.New("RGB11 known transition id mismatch")
	ErrUnrelatedTransition  = errors.New("RGB11 bundle contains an uncommitted transition")
)

type BundleCommitment struct {
	BundleID        [32]byte
	InputOperations map[[32]byte]struct{}
	Transitions     []strict_types.Value
}

// CommitBundle reproduces TransitionBundle::commit_encode. Only inputMap is
// consensus-committed by the BundleId; knownTransitions are disclosures and
// therefore must independently match their OpId and be a subset of inputMap.
func CommitBundle(value strict_types.Value) (BundleCommitment, error) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueStruct {
		return BundleCommitment{}, ErrInvalidBundle
	}
	inputMap, ok := value.Field("inputMap")
	if !ok || inputMap.Unwrap().Kind != strict_types.ValueMap || len(inputMap.Unwrap().Entries) == 0 {
		return BundleCommitment{}, fmt.Errorf("%w: inputMap", ErrInvalidBundle)
	}
	known, ok := value.Field("knownTransitions")
	known = known.Unwrap()
	if !ok || known.Kind != strict_types.ValueList || len(known.Items) == 0 {
		return BundleCommitment{}, fmt.Errorf("%w: knownTransitions", ErrInvalidBundle)
	}

	report := BundleCommitment{
		BundleID:        consensus.TaggedHash(BundleCommitmentTag, inputMap.Encoded),
		InputOperations: make(map[[32]byte]struct{}, len(inputMap.Entries)),
		Transitions:     make([]strict_types.Value, 0, len(known.Items)),
	}
	for _, entry := range inputMap.Entries {
		opid, ok := entry.Value.Bytes()
		if !ok || len(opid) != 32 {
			return BundleCommitment{}, fmt.Errorf("%w: inputMap operation id", ErrInvalidBundle)
		}
		var id [32]byte
		copy(id[:], opid)
		report.InputOperations[id] = struct{}{}
	}
	for _, item := range known.Items {
		item = item.Unwrap()
		declared, declaredOK := item.Field("opid")
		transition, transitionOK := item.Field("transition")
		declaredBytes, bytesOK := declared.Bytes()
		if !declaredOK || !transitionOK || !bytesOK || len(declaredBytes) != 32 {
			return BundleCommitment{}, fmt.Errorf("%w: known transition", ErrInvalidBundle)
		}
		commitment, err := CommitTransition(transition)
		if err != nil {
			return BundleCommitment{}, err
		}
		if !bytes.Equal(declaredBytes, commitment.OperationID[:]) {
			return BundleCommitment{}, ErrTransitionIDMismatch
		}
		if _, ok := report.InputOperations[commitment.OperationID]; !ok {
			return BundleCommitment{}, ErrUnrelatedTransition
		}
		report.Transitions = append(report.Transitions, transition)
	}
	return report, nil
}
