package schemas

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

var (
	ErrInputUnresolved       = errors.New("RGB11 transition input state is unresolved")
	ErrUnsupportedTransition = errors.New("RGB11 standard transition rule is not implemented")
)

type InputRef struct {
	OperationID    [32]byte
	AssignmentType uint64
	Index          uint16
}

type ResolvedInput struct {
	Class  string
	Amount uint64
	Data   []byte
}

type InputResolver interface {
	ResolveRGB11Input(InputRef) (ResolvedInput, error)
}

type InputResolverFunc func(InputRef) (ResolvedInput, error)

func (f InputResolverFunc) ResolveRGB11Input(ref InputRef) (ResolvedInput, error) { return f(ref) }

type TransitionContext struct {
	ContractID      [32]byte
	ContractGlobals map[uint64][][]byte
}

type TransitionValidation struct {
	OperationID      [32]byte
	TransitionType   uint64
	ConformanceValid bool
	StateTypesValid  bool
	InputsResolved   bool
	ScriptValid      bool
	Inputs           int
	Assignments      int
	InputAmounts     map[uint64]uint64
	OutputAmounts    map[uint64]uint64
}

// ValidateTransition validates one transition against the schema and resolved
// prior client-side state. A caller must resolve every Opout from already
// validated history; chain UTXO lookup alone is not sufficient.
func ValidateTransition(
	schema, typeSystem, transition strict_types.Value,
	context TransitionContext,
	resolver InputResolver,
) (TransitionValidation, error) {
	report := TransitionValidation{InputAmounts: make(map[uint64]uint64), OutputAmounts: make(map[uint64]uint64)}
	if resolver == nil {
		return report, ErrInputUnresolved
	}
	schemaID, err := ID(schema)
	if err != nil {
		return report, err
	}
	descriptor, err := ByID(schemaID)
	if err != nil {
		return report, err
	}
	commitment, err := operations.CommitTransition(transition)
	if err != nil {
		return report, err
	}
	report.OperationID = commitment.OperationID

	contractValue, ok := transition.Field("contractId")
	contractID, contractOK := contractValue.Bytes()
	if !ok || !contractOK || len(contractID) != 32 || !bytes.Equal(contractID, context.ContractID[:]) {
		return report, fmt.Errorf("%w: transition contract id", ErrSchemaConformance)
	}
	typeValue, ok := transition.Field("transitionType")
	typeID, typeOK := number(typeValue)
	if !ok || !typeOK {
		return report, fmt.Errorf("%w: transition type", ErrSchemaConformance)
	}
	report.TransitionType = typeID
	transitionDetails, err := mapByNumberField(schema, "transitions")
	if err != nil {
		return report, err
	}
	details, ok := transitionDetails[typeID]
	if !ok {
		return report, fmt.Errorf("%w: undeclared transition %d", ErrSchemaConformance, typeID)
	}
	transitionSchema, ok := details.Field("transitionSchema")
	if !ok {
		return report, fmt.Errorf("%w: transition schema %d", ErrSchemaConformance, typeID)
	}

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return report, err
	}
	if err := registry.AddTypeSystem("contract", typeSystem); err != nil {
		return report, fmt.Errorf("%w: %v", ErrStateType, err)
	}
	if err := validateMetadata(registry, schema, transitionSchema, transition); err != nil {
		return report, err
	}
	metadataAmounts, err := decodeTransitionMetadata(registry, schema, transition)
	if err != nil {
		return report, err
	}
	transitionGlobals, _, err := validateGlobals(registry, schema, transitionSchema, transition)
	if err != nil {
		return report, err
	}
	outputs, count, err := validateAssignments(registry, schema, transitionSchema, transition)
	if err != nil {
		return report, err
	}
	report.Assignments = count
	for id, state := range outputs {
		if state.fungible {
			report.OutputAmounts[id] = state.sum
		}
	}

	inputConstraints, err := occurrenceMap(transitionSchema, "inputs")
	if err != nil {
		return report, err
	}
	ownedDetails, err := mapByNumberField(schema, "ownedTypes")
	if err != nil {
		return report, err
	}
	inputsValue, ok := transition.Field("inputs")
	inputs, inputsOK := sequence(inputsValue)
	if !ok || !inputsOK {
		return report, fmt.Errorf("%w: transition inputs", ErrSchemaConformance)
	}
	inputCounts := make(map[uint64]uint64)
	inputClasses := make(map[uint64]string)
	inputData := make(map[uint64][][]byte)
	for _, input := range inputs {
		input = input.Unwrap()
		opValue, opOK := input.Field("op")
		typeValue, typeOK := input.Field("ty")
		indexValue, indexOK := input.Field("no")
		op, bytesOK := opValue.Bytes()
		assignmentType, numberOK := number(typeValue)
		index, indexNumberOK := number(indexValue)
		if !opOK || !typeOK || !indexOK || !bytesOK || len(op) != 32 || !numberOK || !indexNumberOK || index > math.MaxUint16 {
			return report, fmt.Errorf("%w: input opout", ErrSchemaConformance)
		}
		if _, ok := inputConstraints[assignmentType]; !ok {
			return report, fmt.Errorf("%w: undeclared input assignment %d", ErrSchemaConformance, assignmentType)
		}
		details, ok := ownedDetails[assignmentType]
		if !ok {
			return report, fmt.Errorf("%w: input assignment type %d absent", ErrSchemaConformance, assignmentType)
		}
		stateSchema, ok := details.Field("ownedStateSchema")
		stateSchema = stateSchema.Unwrap()
		if !ok || stateSchema.Kind != strict_types.ValueUnion {
			return report, fmt.Errorf("%w: input state schema %d", ErrSchemaConformance, assignmentType)
		}
		var operationID [32]byte
		copy(operationID[:], op)
		state, err := resolver.ResolveRGB11Input(InputRef{OperationID: operationID, AssignmentType: assignmentType, Index: uint16(index)})
		if err != nil {
			return report, fmt.Errorf("%w: %x/%d/%d: %v", ErrInputUnresolved, operationID, assignmentType, index, err)
		}
		if state.Class != stateSchema.Name {
			return report, fmt.Errorf("%w: input state class %d", ErrStateType, assignmentType)
		}
		inputClasses[assignmentType] = state.Class
		if state.Class == "fungible" {
			if math.MaxUint64-report.InputAmounts[assignmentType] < state.Amount {
				return report, fmt.Errorf("%w: input amount overflow %d", ErrStateType, assignmentType)
			}
			report.InputAmounts[assignmentType] += state.Amount
		} else if state.Class == "structured" {
			inputData[assignmentType] = append(inputData[assignmentType], append([]byte(nil), state.Data...))
		}
		inputCounts[assignmentType]++
		report.Inputs++
	}
	if err := enforceOccurrences("input", inputConstraints, inputCounts); err != nil {
		return report, err
	}
	report.ConformanceValid = true
	report.StateTypesValid = true
	report.InputsResolved = true

	if err := validateTransitionScript(descriptor.Kind, typeID, transition, commitment.OperationID, context,
		metadataAmounts, transitionGlobals, inputCounts, report.InputAmounts, inputData, outputs); err != nil {
		return report, err
	}
	report.ScriptValid = true
	_ = inputClasses
	return report, nil
}

func validateTransitionScript(
	kind Kind,
	transitionType uint64,
	transition strict_types.Value,
	operationID [32]byte,
	context TransitionContext,
	metadata map[uint64]uint64,
	globals map[uint64][]decodedGlobal,
	inputCounts map[uint64]uint64,
	inputAmounts map[uint64]uint64,
	inputData map[uint64][][]byte,
	outputs map[uint64]decodedOwned,
) error {
	switch kind {
	case NIA, CFA:
		if transitionType != 10000 || inputAmounts[ownedAsset] != outputs[ownedAsset].sum {
			return fmt.Errorf("%w: fungible input/output sum", ErrSchemaScript)
		}
	case PFA:
		if transitionType != 10000 || inputAmounts[ownedAsset] != outputs[ownedAsset].sum {
			return fmt.Errorf("%w: permissioned fungible input/output sum", ErrSchemaScript)
		}
		if err := verifyPfaSignature(transition, operationID, context.ContractGlobals[3006]); err != nil {
			return err
		}
	case IFA:
		switch transitionType {
		case 10000:
			if inputAmounts[ownedAsset] != outputs[ownedAsset].sum ||
				inputAmounts[ownedInflation] != outputs[ownedInflation].sum ||
				inputCounts[4013] != outputs[4013].count {
				return fmt.Errorf("%w: IFA transfer conservation", ErrSchemaScript)
			}
		case 8000:
			issued, err := oneGlobalAmount(globals, globalIssuedSupply)
			if err != nil {
				return err
			}
			remaining, ok := metadata[1000]
			if !ok || issued != outputs[ownedAsset].sum || remaining != outputs[ownedInflation].sum ||
				inputAmounts[ownedInflation] != issued+remaining || issued+remaining < issued {
				return fmt.Errorf("%w: IFA inflation allowance", ErrSchemaScript)
			}
		case 8010:
			burnedAsset, assetOK := metadata[1001]
			burnedInflation, inflationOK := metadata[1002]
			assetTotal := outputs[ownedAsset].sum + burnedAsset
			inflationTotal := outputs[ownedInflation].sum + burnedInflation
			if !assetOK || !inflationOK ||
				assetTotal < outputs[ownedAsset].sum || inflationTotal < outputs[ownedInflation].sum ||
				inputAmounts[ownedAsset] != assetTotal || inputAmounts[ownedInflation] != inflationTotal ||
				(inputAmounts[ownedAsset] > 0 && burnedAsset == 0) ||
				(inputAmounts[ownedInflation] > 0 && burnedInflation == 0) ||
				(burnedAsset == 0 && burnedInflation == 0) {
				return fmt.Errorf("%w: IFA burn accounting", ErrSchemaScript)
			}
		default:
			return fmt.Errorf("%w: IFA transition %d", ErrUnsupportedTransition, transitionType)
		}
	case UDA:
		if transitionType != 10000 || len(inputData[ownedAsset]) != 1 || len(inputData[ownedAsset][0]) < 4 ||
			len(outputs[ownedAsset].raw) != 1 || len(outputs[ownedAsset].raw[0]) < 12 ||
			!bytes.Equal(inputData[ownedAsset][0][:4], outputs[ownedAsset].raw[0][:4]) ||
			binaryLittleEndianUint64(outputs[ownedAsset].raw[0][4:12]) != 1 {
			return fmt.Errorf("%w: unique asset transfer", ErrSchemaScript)
		}
	default:
		return ErrUnknownSchema
	}
	return nil
}

func decodeTransitionMetadata(registry *strict_types.Registry, schema, transition strict_types.Value) (map[uint64]uint64, error) {
	typeDetails, err := mapByNumberField(schema, "metaTypes")
	if err != nil {
		return nil, err
	}
	metadataValue, ok := transition.Field("metadata")
	actual, actualOK := entries(metadataValue)
	if !ok || !actualOK {
		return nil, fmt.Errorf("%w: metadata map", ErrSchemaConformance)
	}
	result := make(map[uint64]uint64, len(actual))
	for _, entry := range actual {
		typeID, idOK := number(entry.Key)
		details, detailsOK := typeDetails[typeID]
		semValue, semOK := details.Field("semId")
		semID, semanticOK := semanticID(semValue)
		raw, rawOK := entry.Value.Bytes()
		if !idOK || !detailsOK || !semOK || !semanticOK || !rawOK {
			return nil, fmt.Errorf("%w: metadata %d", ErrStateType, typeID)
		}
		decoded, err := registry.DecodeSemantic("contract", semID, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: metadata %d: %v", ErrStateType, typeID, err)
		}
		amount, ok := scalarUint(decoded)
		if !ok {
			return nil, fmt.Errorf("%w: metadata amount %d", ErrStateType, typeID)
		}
		result[typeID] = amount
	}
	return result, nil
}

func verifyPfaSignature(transition strict_types.Value, operationID [32]byte, pubkeys [][]byte) error {
	if len(pubkeys) != 1 {
		return fmt.Errorf("%w: permissioned pubkey", ErrSchemaScript)
	}
	pubkey, err := btcec.ParsePubKey(pubkeys[0])
	if err != nil {
		return fmt.Errorf("%w: permissioned pubkey: %v", ErrSchemaScript, err)
	}
	signatureValue, ok := transition.Field("signature")
	signatureValue = signatureValue.Unwrap()
	if !ok || signatureValue.Kind != strict_types.ValueUnion || signatureValue.Name != "some" || signatureValue.Inner == nil {
		return fmt.Errorf("%w: permissioned transition signature missing", ErrSchemaScript)
	}
	compact, ok := signatureValue.Inner.Bytes()
	if !ok || len(compact) != 64 {
		return fmt.Errorf("%w: permissioned transition signature", ErrSchemaScript)
	}
	var r, s btcec.ModNScalar
	if r.SetByteSlice(compact[:32]) || s.SetByteSlice(compact[32:]) || r.IsZero() || s.IsZero() {
		return fmt.Errorf("%w: permissioned transition signature scalar", ErrSchemaScript)
	}
	if !ecdsa.NewSignature(&r, &s).Verify(operationID[:], pubkey) {
		return fmt.Errorf("%w: permissioned transition signature invalid", ErrSchemaScript)
	}
	return nil
}

func binaryLittleEndianUint64(data []byte) uint64 {
	return uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
		uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56
}
