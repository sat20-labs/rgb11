package consignment

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

var ErrMergeHistory = errors.New("RGB11 consignment histories cannot be merged")

// MergeHistories combines compatible client-side histories for the same
// contract. Shared bundles are deduplicated by BundleId and complementary
// revealed seal disclosures are retained. The resulting terminal set is
// intentionally empty: BuildTransfer replaces it with the new recipient or
// wallet-change terminals.
func MergeHistories(containers ...*Container) (*Container, error) {
	if len(containers) == 0 || containers[0] == nil || !containers[0].StructuralValid {
		return nil, ErrMergeHistory
	}
	base := containers[0]
	value := base.Value.Clone()
	bundles := fieldPointer(&value, "bundles")
	terminals := fieldPointer(&value, "terminals")
	transfer := fieldPointer(&value, "transfer")
	if bundles == nil || terminals == nil || transfer == nil {
		return nil, ErrMergeHistory
	}
	bundles = unwrapPointer(bundles)
	terminals = unwrapPointer(terminals)
	if bundles.Kind != strict_types.ValueList || terminals.Kind != strict_types.ValueMap {
		return nil, ErrMergeHistory
	}
	*transfer = strict_types.Value{Kind: strict_types.ValueEnum, Name: "true", Text: "true", Tag: 1}
	terminals.Entries = nil

	index := make(map[[32]byte]int)
	for position := range bundles.Items {
		id, err := witnessBundleID(bundles.Items[position])
		if err != nil {
			return nil, err
		}
		if _, duplicate := index[id]; duplicate {
			return nil, ErrMergeHistory
		}
		index[id] = position
	}

	for _, candidate := range containers[1:] {
		if candidate == nil || !candidate.StructuralValid || candidate.ContractID != base.ContractID ||
			candidate.SchemaID != base.SchemaID || !sameStaticConsignmentFields(base.Value, candidate.Value) {
			return nil, ErrMergeHistory
		}
		candidateBundles, ok := candidate.Value.Field("bundles")
		candidateBundles = candidateBundles.Unwrap()
		if !ok || candidateBundles.Kind != strict_types.ValueList {
			return nil, ErrMergeHistory
		}
		for _, item := range candidateBundles.Items {
			id, err := witnessBundleID(item)
			if err != nil {
				return nil, err
			}
			if position, found := index[id]; found {
				if err := mergeWitnessBundleDisclosures(&bundles.Items[position], item); err != nil {
					return nil, err
				}
				continue
			}
			if len(bundles.Items) >= int(^uint32(0)) {
				return nil, ErrMergeHistory
			}
			bundles.Items = append(bundles.Items, item.Clone())
			index[id] = len(bundles.Items) - 1
		}
	}

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return nil, err
	}
	encoded, err := registry.Encode("RGBStd", "Consignmenttrue", value)
	if err != nil {
		return nil, err
	}
	rebuilt, err := registry.Decode("RGBStd", "Consignmenttrue", encoded)
	if err != nil {
		return nil, err
	}
	result := *base
	result.Value = rebuilt
	result.Armor = nil
	result.ConsensusValid = false
	return &result, nil
}

func sameStaticConsignmentFields(first, second strict_types.Value) bool {
	for _, name := range []string{"version", "genesis", "schema", "types", "scripts"} {
		left, leftOK := first.Field(name)
		right, rightOK := second.Field(name)
		if !leftOK || !rightOK || !bytes.Equal(left.Encoded, right.Encoded) {
			return false
		}
	}
	return true
}

func witnessBundleID(value strict_types.Value) ([32]byte, error) {
	bundle, ok := value.Unwrap().Field("bundle")
	if !ok {
		return [32]byte{}, ErrMergeHistory
	}
	commitment, err := operations.CommitBundle(bundle)
	if err != nil {
		return [32]byte{}, err
	}
	return commitment.BundleID, nil
}

func mergeWitnessBundleDisclosures(destination *strict_types.Value, source strict_types.Value) error {
	dst := unwrapPointer(destination)
	src := source.Unwrap()
	for _, name := range []string{"pubWitness", "anchor"} {
		left, leftOK := dst.Field(name)
		right, rightOK := src.Field(name)
		if !leftOK || !rightOK || !bytes.Equal(left.Encoded, right.Encoded) {
			return fmt.Errorf("%w: shared bundle has a different %s", ErrMergeHistory, name)
		}
	}
	dstBundle := fieldPointer(dst, "bundle")
	srcBundle, ok := src.Field("bundle")
	if dstBundle == nil || !ok {
		return ErrMergeHistory
	}
	dstTransitions := fieldPointer(unwrapPointer(dstBundle), "knownTransitions")
	srcTransitions, ok := srcBundle.Unwrap().Field("knownTransitions")
	if dstTransitions == nil || !ok {
		return ErrMergeHistory
	}
	dstTransitions = unwrapPointer(dstTransitions)
	srcTransitions = srcTransitions.Unwrap()
	if dstTransitions.Kind != strict_types.ValueList || srcTransitions.Kind != strict_types.ValueList ||
		len(dstTransitions.Items) != len(srcTransitions.Items) {
		return ErrMergeHistory
	}
	byID := make(map[[32]byte]int, len(dstTransitions.Items))
	for index, item := range dstTransitions.Items {
		transition, ok := item.Unwrap().Field("transition")
		if !ok {
			return ErrMergeHistory
		}
		commitment, err := operations.CommitTransition(transition)
		if err != nil {
			return err
		}
		byID[commitment.OperationID] = index
	}
	for _, item := range srcTransitions.Items {
		transition, ok := item.Unwrap().Field("transition")
		if !ok {
			return ErrMergeHistory
		}
		commitment, err := operations.CommitTransition(transition)
		if err != nil {
			return err
		}
		position, ok := byID[commitment.OperationID]
		if !ok {
			return ErrMergeHistory
		}
		destinationTransition := fieldPointer(unwrapPointer(&dstTransitions.Items[position]), "transition")
		if destinationTransition == nil || mergeOperationDisclosures(destinationTransition, transition) != nil {
			return ErrMergeHistory
		}
	}
	return nil
}

func mergeOperationDisclosures(destination *strict_types.Value, source strict_types.Value) error {
	dstAssignments := fieldPointer(unwrapPointer(destination), "assignments")
	srcAssignments, ok := source.Unwrap().Field("assignments")
	if dstAssignments == nil || !ok {
		return ErrMergeHistory
	}
	dstAssignments = unwrapPointer(dstAssignments)
	srcAssignments = srcAssignments.Unwrap()
	if dstAssignments.Kind != strict_types.ValueMap || srcAssignments.Kind != strict_types.ValueMap ||
		len(dstAssignments.Entries) != len(srcAssignments.Entries) {
		return ErrMergeHistory
	}
	for entryIndex := range dstAssignments.Entries {
		dstEntry := &dstAssignments.Entries[entryIndex]
		srcEntry := srcAssignments.Entries[entryIndex]
		if !bytes.Equal(dstEntry.Key.Encoded, srcEntry.Key.Encoded) {
			return ErrMergeHistory
		}
		dstTyped := unwrapPointer(&dstEntry.Value)
		srcTyped := srcEntry.Value.Unwrap()
		if dstTyped.Kind != strict_types.ValueUnion || srcTyped.Kind != strict_types.ValueUnion ||
			dstTyped.Tag != srcTyped.Tag || dstTyped.Inner == nil || srcTyped.Inner == nil {
			return ErrMergeHistory
		}
		dstList := unwrapPointer(dstTyped.Inner)
		srcList := srcTyped.Inner.Unwrap()
		if dstList.Kind != strict_types.ValueList || srcList.Kind != strict_types.ValueList || len(dstList.Items) != len(srcList.Items) {
			return ErrMergeHistory
		}
		for itemIndex := range dstList.Items {
			dstItem := unwrapPointer(&dstList.Items[itemIndex])
			srcItem := srcList.Items[itemIndex].Unwrap()
			if dstItem.Kind != strict_types.ValueUnion || srcItem.Kind != strict_types.ValueUnion {
				return ErrMergeHistory
			}
			if dstItem.Name == "confidentialSeal" && srcItem.Name == "revealed" {
				*dstItem = srcItem.Clone()
				continue
			}
			if dstItem.Name == "revealed" && srcItem.Name == "confidentialSeal" {
				continue
			}
			if dstItem.Name != srcItem.Name || !bytes.Equal(dstItem.Encoded, srcItem.Encoded) {
				return ErrMergeHistory
			}
		}
	}
	return nil
}
