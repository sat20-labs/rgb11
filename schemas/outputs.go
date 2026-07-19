package schemas

import (
	"fmt"
	"math"

	"github.com/sat20-labs/rgb11/strict_types"
)

// RevealedOutput is client-side state which can be used as an input by a
// subsequent transition. Concealed assignments are intentionally omitted:
// they cannot be projected or spent until the receiver reveals them.
type RevealedOutput struct {
	AssignmentType uint64
	Index          uint16
	State          ResolvedInput
	Seal           strict_types.Value
}

// RevealedOutputs extracts disclosed state and its seal from a genesis or
// transition after schema validation. It does not itself establish validity.
func RevealedOutputs(operation strict_types.Value) ([]RevealedOutput, error) {
	assignmentsValue, ok := operation.Field("assignments")
	assignmentEntries, entriesOK := entries(assignmentsValue)
	if !ok || !entriesOK {
		return nil, fmt.Errorf("%w: assignments", ErrSchemaConformance)
	}
	var outputs []RevealedOutput
	for _, entry := range assignmentEntries {
		assignmentType, ok := number(entry.Key)
		typed := entry.Value.Unwrap()
		if !ok || typed.Kind != strict_types.ValueUnion || typed.Inner == nil {
			return nil, fmt.Errorf("%w: typed assignment", ErrSchemaConformance)
		}
		items, ok := sequence(*typed.Inner)
		if !ok || len(items) > math.MaxUint16+1 {
			return nil, fmt.Errorf("%w: assignment list", ErrSchemaConformance)
		}
		for index, item := range items {
			item = item.Unwrap()
			if item.Kind != strict_types.ValueUnion || item.Name != "revealed" || item.Inner == nil {
				continue
			}
			fields := item.Inner.Unwrap()
			seal, sealOK := fields.Field("seal")
			stateValue, stateOK := fields.Field("state")
			if !sealOK || !stateOK {
				return nil, fmt.Errorf("%w: revealed assignment", ErrSchemaConformance)
			}
			state := ResolvedInput{Class: typed.Name}
			switch typed.Name {
			case "declarative":
				if stateValue.Unwrap().Kind != strict_types.ValueUnit {
					return nil, fmt.Errorf("%w: declarative assignment", ErrStateType)
				}
			case "fungible":
				amount, ok := fungibleAmount(stateValue)
				if !ok {
					return nil, fmt.Errorf("%w: fungible assignment", ErrStateType)
				}
				state.Amount = amount
			case "structured":
				raw, ok := stateValue.Bytes()
				if !ok {
					return nil, fmt.Errorf("%w: structured assignment", ErrStateType)
				}
				state.Data = raw
			default:
				return nil, fmt.Errorf("%w: assignment class %q", ErrSchemaConformance, typed.Name)
			}
			outputs = append(outputs, RevealedOutput{
				AssignmentType: assignmentType,
				Index:          uint16(index),
				State:          state,
				Seal:           seal,
			})
		}
	}
	return outputs, nil
}
