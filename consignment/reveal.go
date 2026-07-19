package consignment

import (
	"bytes"

	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

type graphDisclosure struct {
	secret [32]byte
	seal   strict_types.Value
}

// RevealGraphSeals applies wallet-owned terminal seal disclosures to decoded
// transition assignments. Sender consignments keep terminal seals concealed;
// the receiving wallet enriches its local copy before consensus validation,
// matching Consignment::reveal_terminal_seals in the frozen Rust stack.
func (c *Container) RevealGraphSeals(reveals []seals.GraphBlindSeal) (int, error) {
	if c == nil || len(reveals) == 0 {
		return 0, nil
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return 0, err
	}
	items := make([]graphDisclosure, 0, len(reveals))
	for _, reveal := range reveals {
		secret, err := reveal.Conceal()
		if err != nil {
			return 0, err
		}
		encoded, err := reveal.StrictBytes()
		if err != nil {
			return 0, err
		}
		value, err := registry.Decode("BPCore", "BlindSealTxPtr", encoded)
		if err != nil {
			return 0, err
		}
		items = append(items, graphDisclosure{secret: [32]byte(secret), seal: value})
	}

	count := 0
	bundles := fieldPointer(&c.Value, "bundles")
	if bundles == nil {
		return 0, ErrContainerType
	}
	bundles = unwrapPointer(bundles)
	if bundles.Kind != strict_types.ValueList {
		return 0, ErrContainerType
	}
	for bundleIndex := range bundles.Items {
		witnessBundle := unwrapPointer(&bundles.Items[bundleIndex])
		bundle := fieldPointer(witnessBundle, "bundle")
		if bundle == nil {
			return 0, ErrContainerType
		}
		known := fieldPointer(unwrapPointer(bundle), "knownTransitions")
		if known == nil {
			return 0, ErrContainerType
		}
		known = unwrapPointer(known)
		for transitionIndex := range known.Items {
			knownTransition := unwrapPointer(&known.Items[transitionIndex])
			transition := fieldPointer(knownTransition, "transition")
			if transition == nil {
				return 0, ErrContainerType
			}
			revealed, err := revealOperationAssignments(transition, items)
			if err != nil {
				return 0, err
			}
			count += revealed
		}
	}
	if count == 0 {
		return 0, nil
	}
	typeName := "Consignmentfalse"
	transfer, _ := c.Value.Field("transfer")
	if yes, ok := transfer.Bool(); ok && yes {
		typeName = "Consignmenttrue"
	}
	encoded, err := registry.Encode("RGBStd", typeName, c.Value)
	if err != nil {
		return 0, err
	}
	rebuilt, err := registry.Decode("RGBStd", typeName, encoded)
	if err != nil {
		return 0, err
	}
	c.Value = rebuilt
	return count, nil
}

func revealOperationAssignments(operation *strict_types.Value, reveals []graphDisclosure) (int, error) {
	assignments := fieldPointer(unwrapPointer(operation), "assignments")
	if assignments == nil {
		return 0, ErrContainerType
	}
	assignments = unwrapPointer(assignments)
	if assignments.Kind != strict_types.ValueMap {
		return 0, ErrContainerType
	}
	count := 0
	for entryIndex := range assignments.Entries {
		typed := unwrapPointer(&assignments.Entries[entryIndex].Value)
		if typed.Kind != strict_types.ValueUnion || typed.Inner == nil {
			return 0, ErrContainerType
		}
		list := unwrapPointer(typed.Inner)
		for assignmentIndex := range list.Items {
			assignment := unwrapPointer(&list.Items[assignmentIndex])
			if assignment.Kind != strict_types.ValueUnion || assignment.Name != "confidentialSeal" || assignment.Inner == nil {
				continue
			}
			fields := unwrapPointer(assignment.Inner)
			secretValue := fieldPointer(fields, "seal")
			stateValue := fieldPointer(fields, "state")
			if secretValue == nil || stateValue == nil {
				return 0, ErrContainerType
			}
			secret, ok := secretValue.Bytes()
			if !ok || len(secret) != 32 {
				return 0, ErrContainerType
			}
			for _, reveal := range reveals {
				if !bytes.Equal(secret, reveal.secret[:]) {
					continue
				}
				inner := strict_types.Value{Kind: strict_types.ValueStruct, Fields: []strict_types.Field{
					{Name: "seal", Value: reveal.seal},
					{Name: "state", Value: *stateValue},
				}}
				*assignment = strict_types.Value{Kind: strict_types.ValueUnion, Name: "revealed", Tag: 0, Inner: &inner}
				count++
				break
			}
		}
	}
	return count, nil
}

func fieldPointer(value *strict_types.Value, name string) *strict_types.Value {
	value = unwrapPointer(value)
	if value == nil || value.Kind != strict_types.ValueStruct {
		return nil
	}
	for index := range value.Fields {
		if value.Fields[index].Name == name {
			return &value.Fields[index].Value
		}
	}
	return nil
}

func unwrapPointer(value *strict_types.Value) *strict_types.Value {
	for value != nil && value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		value = &value.Items[0]
	}
	return value
}
