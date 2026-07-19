package schemas

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-schemas
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: c5e43e987d18a2398d5f5f6c78629480fd792abd
// Upstream-File: src/nia.rs
// Upstream-File: src/cfa.rs
// Upstream-File: src/ifa.rs
// Upstream-File: src/pfa.rs
// Upstream-File: src/uda.rs
// Translation-Revision: 1

var (
	ErrSchemaConformance = errors.New("RGB11 operation does not conform to schema")
	ErrStateType         = errors.New("RGB11 state fails strict semantic type validation")
	ErrSchemaScript      = errors.New("RGB11 standard schema validation script failed")
)

const (
	globalIssuedSupply uint64 = 2010
	globalMaxSupply    uint64 = 2011
	globalTokens       uint64 = 2102
	ownedAsset         uint64 = 4000
	ownedInflation     uint64 = 4010
)

// GenesisValidation reports the independently checked layers of a genesis.
// It does not make any Bitcoin witness/anchor claim.
type GenesisValidation struct {
	SchemaID         string
	Kind             Kind
	ConformanceValid bool
	StateTypesValid  bool
	ScriptValid      bool
	GlobalStates     int
	Assignments      int
	FungibleAmounts  map[uint64]uint64
}

// ValidateGenesis verifies occurrence constraints, rejects undeclared state,
// decodes metadata/global/structured owned state using the TypeSystem carried
// by the consignment, and executes the frozen standard-schema genesis rules.
// Bitcoin seal and witness validation remains a separate mandatory layer.
func ValidateGenesis(schema, typeSystem, genesis strict_types.Value) (GenesisValidation, error) {
	schemaID, err := ID(schema)
	if err != nil {
		return GenesisValidation{}, err
	}
	descriptor, err := ByID(schemaID)
	if err != nil {
		return GenesisValidation{}, err
	}
	report := GenesisValidation{SchemaID: schemaID, Kind: descriptor.Kind, FungibleAmounts: make(map[uint64]uint64)}

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return report, err
	}
	if err := registry.AddTypeSystem("contract", typeSystem); err != nil {
		return report, fmt.Errorf("%w: %v", ErrStateType, err)
	}

	genesisSchema, ok := schema.Field("genesis")
	if !ok {
		return report, fmt.Errorf("%w: genesis schema missing", ErrSchemaConformance)
	}
	if err := validateMetadata(registry, schema, genesisSchema, genesis); err != nil {
		return report, err
	}
	decodedGlobals, count, err := validateGlobals(registry, schema, genesisSchema, genesis)
	if err != nil {
		return report, err
	}
	report.GlobalStates = count
	owned, count, err := validateAssignments(registry, schema, genesisSchema, genesis)
	if err != nil {
		return report, err
	}
	report.Assignments = count
	for typeID, state := range owned {
		if state.fungible {
			report.FungibleAmounts[typeID] = state.sum
		}
	}
	report.ConformanceValid = true
	report.StateTypesValid = true

	if err := validateGenesisScript(descriptor.Kind, decodedGlobals, owned); err != nil {
		return report, err
	}
	report.ScriptValid = true
	return report, nil
}

type decodedGlobal struct {
	raw     []byte
	decoded strict_types.Value
}

type decodedOwned struct {
	kind     string
	fungible bool
	count    uint64
	sum      uint64
	raw      [][]byte
}

func validateMetadata(registry *strict_types.Registry, schema, genesisSchema, genesis strict_types.Value) error {
	allowedValue, ok := genesisSchema.Field("metadata")
	if !ok {
		return fmt.Errorf("%w: metadata occurrence schema missing", ErrSchemaConformance)
	}
	allowedItems, ok := sequence(allowedValue)
	if !ok {
		return fmt.Errorf("%w: metadata occurrence schema", ErrSchemaConformance)
	}
	allowed := make(map[uint64]struct{}, len(allowedItems))
	for _, item := range allowedItems {
		id, ok := number(item)
		if !ok {
			return fmt.Errorf("%w: metadata type", ErrSchemaConformance)
		}
		allowed[id] = struct{}{}
	}
	typeDetails, err := mapByNumberField(schema, "metaTypes")
	if err != nil {
		return err
	}
	actualValue, ok := genesis.Field("metadata")
	if !ok {
		return fmt.Errorf("%w: metadata missing", ErrSchemaConformance)
	}
	actual, ok := entries(actualValue)
	if !ok {
		return fmt.Errorf("%w: metadata map", ErrSchemaConformance)
	}
	for _, entry := range actual {
		id, ok := number(entry.Key)
		if !ok {
			return fmt.Errorf("%w: metadata type", ErrSchemaConformance)
		}
		if _, ok := allowed[id]; !ok {
			return fmt.Errorf("%w: undeclared metadata type %d", ErrSchemaConformance, id)
		}
		details, ok := typeDetails[id]
		if !ok {
			return fmt.Errorf("%w: metadata type %d absent", ErrSchemaConformance, id)
		}
		semIDValue, ok := details.Field("semId")
		if !ok {
			return fmt.Errorf("%w: metadata semantic id", ErrSchemaConformance)
		}
		semID, ok := semanticID(semIDValue)
		raw, rawOK := entry.Value.Bytes()
		if !ok || !rawOK {
			return fmt.Errorf("%w: metadata %d", ErrStateType, id)
		}
		if _, err := registry.DecodeSemantic("contract", semID, raw); err != nil {
			return fmt.Errorf("%w: metadata %d: %v", ErrStateType, id, err)
		}
	}
	return nil
}

func validateGlobals(registry *strict_types.Registry, schema, genesisSchema, genesis strict_types.Value) (map[uint64][]decodedGlobal, int, error) {
	typeDetails, err := mapByNumberField(schema, "globalTypes")
	if err != nil {
		return nil, 0, err
	}
	constraints, err := occurrenceMap(genesisSchema, "globals")
	if err != nil {
		return nil, 0, err
	}
	actualValue, ok := genesis.Field("globals")
	if !ok {
		return nil, 0, fmt.Errorf("%w: globals missing", ErrSchemaConformance)
	}
	actualEntries, ok := entries(actualValue)
	if !ok {
		return nil, 0, fmt.Errorf("%w: globals map", ErrSchemaConformance)
	}
	actualCounts := make(map[uint64]uint64)
	result := make(map[uint64][]decodedGlobal)
	total := 0
	for _, entry := range actualEntries {
		id, ok := number(entry.Key)
		if !ok {
			return nil, 0, fmt.Errorf("%w: global state type", ErrSchemaConformance)
		}
		if _, ok := constraints[id]; !ok {
			return nil, 0, fmt.Errorf("%w: undeclared global state %d", ErrSchemaConformance, id)
		}
		details, ok := typeDetails[id]
		if !ok {
			return nil, 0, fmt.Errorf("%w: global state type %d absent", ErrSchemaConformance, id)
		}
		stateSchema, ok := details.Field("globalStateSchema")
		if !ok {
			return nil, 0, fmt.Errorf("%w: global state schema %d", ErrSchemaConformance, id)
		}
		semValue, ok := stateSchema.Unwrap().Field("semId")
		semID, semOK := semanticID(semValue)
		states, statesOK := sequence(entry.Value)
		if !ok || !semOK || !statesOK {
			return nil, 0, fmt.Errorf("%w: global state %d", ErrSchemaConformance, id)
		}
		for _, state := range states {
			raw, ok := state.Bytes()
			if !ok {
				return nil, 0, fmt.Errorf("%w: global state %d payload", ErrStateType, id)
			}
			decoded, err := registry.DecodeSemantic("contract", semID, raw)
			if err != nil {
				return nil, 0, fmt.Errorf("%w: global state %d: %v", ErrStateType, id, err)
			}
			result[id] = append(result[id], decodedGlobal{raw: raw, decoded: decoded})
			total++
		}
		actualCounts[id] = uint64(len(states))
	}
	if err := enforceOccurrences("global", constraints, actualCounts); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func validateAssignments(registry *strict_types.Registry, schema, genesisSchema, genesis strict_types.Value) (map[uint64]decodedOwned, int, error) {
	typeDetails, err := mapByNumberField(schema, "ownedTypes")
	if err != nil {
		return nil, 0, err
	}
	constraints, err := occurrenceMap(genesisSchema, "assignments")
	if err != nil {
		return nil, 0, err
	}
	actualValue, ok := genesis.Field("assignments")
	if !ok {
		return nil, 0, fmt.Errorf("%w: assignments missing", ErrSchemaConformance)
	}
	actualEntries, ok := entries(actualValue)
	if !ok {
		return nil, 0, fmt.Errorf("%w: assignments map", ErrSchemaConformance)
	}
	actualCounts := make(map[uint64]uint64)
	result := make(map[uint64]decodedOwned)
	total := 0
	for _, entry := range actualEntries {
		id, ok := number(entry.Key)
		if !ok {
			return nil, 0, fmt.Errorf("%w: assignment type", ErrSchemaConformance)
		}
		if _, ok := constraints[id]; !ok {
			return nil, 0, fmt.Errorf("%w: undeclared assignment %d", ErrSchemaConformance, id)
		}
		details, ok := typeDetails[id]
		if !ok {
			return nil, 0, fmt.Errorf("%w: assignment type %d absent", ErrSchemaConformance, id)
		}
		ownedSchema, ok := details.Field("ownedStateSchema")
		ownedSchema = ownedSchema.Unwrap()
		typed := entry.Value.Unwrap()
		if !ok || ownedSchema.Kind != strict_types.ValueUnion || typed.Kind != strict_types.ValueUnion ||
			ownedSchema.Name != typed.Name || ownedSchema.Inner == nil || typed.Inner == nil {
			return nil, 0, fmt.Errorf("%w: assignment state class %d", ErrSchemaConformance, id)
		}
		assignments, ok := sequence(*typed.Inner)
		if !ok {
			return nil, 0, fmt.Errorf("%w: assignment list %d", ErrSchemaConformance, id)
		}
		state := decodedOwned{kind: typed.Name, fungible: typed.Name == "fungible", count: uint64(len(assignments))}
		var structuredSemID string
		if typed.Name == "structured" {
			structuredSemID, ok = semanticID(*ownedSchema.Inner)
			if !ok {
				return nil, 0, fmt.Errorf("%w: structured assignment semantic id %d", ErrSchemaConformance, id)
			}
		}
		if typed.Name == "fungible" {
			fungibleType := ownedSchema.Inner.Unwrap()
			if fungibleType.Kind != strict_types.ValueEnum || fungibleType.Tag != 8 {
				return nil, 0, fmt.Errorf("%w: unsupported fungible state %d", ErrSchemaConformance, id)
			}
		}
		for _, assignment := range assignments {
			assignment = assignment.Unwrap()
			if assignment.Kind != strict_types.ValueUnion || assignment.Inner == nil {
				return nil, 0, fmt.Errorf("%w: assignment %d", ErrSchemaConformance, id)
			}
			fields := assignment.Inner.Unwrap()
			stateValue, ok := fields.Field("state")
			if !ok {
				return nil, 0, fmt.Errorf("%w: assignment state %d", ErrSchemaConformance, id)
			}
			switch typed.Name {
			case "declarative":
				if stateValue.Unwrap().Kind != strict_types.ValueUnit {
					return nil, 0, fmt.Errorf("%w: declarative assignment %d", ErrStateType, id)
				}
			case "fungible":
				amount, ok := fungibleAmount(stateValue)
				if !ok || math.MaxUint64-state.sum < amount {
					return nil, 0, fmt.Errorf("%w: fungible assignment %d", ErrStateType, id)
				}
				state.sum += amount
			case "structured":
				raw, ok := stateValue.Bytes()
				if !ok {
					return nil, 0, fmt.Errorf("%w: structured assignment %d", ErrStateType, id)
				}
				if _, err := registry.DecodeSemantic("contract", structuredSemID, raw); err != nil {
					return nil, 0, fmt.Errorf("%w: structured assignment %d: %v", ErrStateType, id, err)
				}
				state.raw = append(state.raw, raw)
			default:
				return nil, 0, fmt.Errorf("%w: unknown assignment class %q", ErrSchemaConformance, typed.Name)
			}
			total++
		}
		actualCounts[id] = uint64(len(assignments))
		result[id] = state
	}
	if err := enforceOccurrences("assignment", constraints, actualCounts); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func validateGenesisScript(kind Kind, globals map[uint64][]decodedGlobal, owned map[uint64]decodedOwned) error {
	switch kind {
	case NIA, CFA, PFA:
		issued, err := oneGlobalAmount(globals, globalIssuedSupply)
		if err != nil {
			return err
		}
		if owned[ownedAsset].sum != issued {
			return fmt.Errorf("%w: issued supply %d != asset assignments %d", ErrSchemaScript, issued, owned[ownedAsset].sum)
		}
	case IFA:
		issued, err := oneGlobalAmount(globals, globalIssuedSupply)
		if err != nil {
			return err
		}
		maximum, err := oneGlobalAmount(globals, globalMaxSupply)
		if err != nil {
			return err
		}
		if owned[ownedAsset].sum != issued {
			return fmt.Errorf("%w: issued supply %d != asset assignments %d", ErrSchemaScript, issued, owned[ownedAsset].sum)
		}
		if maximum < issued || owned[ownedInflation].sum != maximum-issued {
			return fmt.Errorf("%w: inflation assignments do not equal max supply minus issued supply", ErrSchemaScript)
		}
	case UDA:
		tokens := globals[globalTokens]
		allocations := owned[ownedAsset].raw
		if len(tokens) != 1 || len(tokens[0].raw) < 4 || len(allocations) != 1 || len(allocations[0]) < 12 {
			return fmt.Errorf("%w: unique asset token/allocation cardinality", ErrSchemaScript)
		}
		if string(tokens[0].raw[:4]) != string(allocations[0][:4]) || binary.LittleEndian.Uint64(allocations[0][4:12]) != 1 {
			return fmt.Errorf("%w: unique asset token index or fraction", ErrSchemaScript)
		}
	default:
		return ErrUnknownSchema
	}
	return nil
}

func oneGlobalAmount(globals map[uint64][]decodedGlobal, typeID uint64) (uint64, error) {
	values := globals[typeID]
	if len(values) != 1 {
		return 0, fmt.Errorf("%w: global amount %d cardinality", ErrSchemaScript, typeID)
	}
	amount, ok := scalarUint(values[0].decoded)
	if !ok {
		return 0, fmt.Errorf("%w: global amount %d", ErrSchemaScript, typeID)
	}
	return amount, nil
}

func scalarUint(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if amount, ok := value.Uint64(); ok {
		return amount, true
	}
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return scalarUint(*value.Inner)
	}
	return 0, false
}

func fungibleAmount(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueUnion || value.Tag != 8 || value.Inner == nil {
		return 0, false
	}
	return scalarUint(*value.Inner)
}

type occurrence struct{ min, max uint64 }

func occurrenceMap(schema strict_types.Value, field string) (map[uint64]occurrence, error) {
	value, ok := schema.Field(field)
	if !ok {
		return nil, fmt.Errorf("%w: %s constraints missing", ErrSchemaConformance, field)
	}
	items, ok := entries(value)
	if !ok {
		return nil, fmt.Errorf("%w: %s constraints", ErrSchemaConformance, field)
	}
	result := make(map[uint64]occurrence, len(items))
	for _, entry := range items {
		id, idOK := number(entry.Key)
		bounds := entry.Value.Unwrap()
		minValue, minOK := bounds.Field("min")
		maxValue, maxOK := bounds.Field("max")
		min, minUint := number(minValue)
		max, maxUint := number(maxValue)
		if !idOK || !minOK || !maxOK || !minUint || !maxUint || min > max {
			return nil, fmt.Errorf("%w: %s occurrence", ErrSchemaConformance, field)
		}
		result[id] = occurrence{min: min, max: max}
	}
	return result, nil
}

func enforceOccurrences(label string, constraints map[uint64]occurrence, actual map[uint64]uint64) error {
	for id, bound := range constraints {
		count := actual[id]
		if count < bound.min || count > bound.max {
			return fmt.Errorf("%w: %s %d count %d outside %d..%d", ErrSchemaConformance, label, id, count, bound.min, bound.max)
		}
	}
	return nil
}

func mapByNumberField(value strict_types.Value, field string) (map[uint64]strict_types.Value, error) {
	mapValue, ok := value.Field(field)
	if !ok {
		return nil, fmt.Errorf("%w: %s missing", ErrSchemaConformance, field)
	}
	items, ok := entries(mapValue)
	if !ok {
		return nil, fmt.Errorf("%w: %s map", ErrSchemaConformance, field)
	}
	result := make(map[uint64]strict_types.Value, len(items))
	for _, entry := range items {
		id, ok := number(entry.Key)
		if !ok {
			return nil, fmt.Errorf("%w: %s key", ErrSchemaConformance, field)
		}
		result[id] = entry.Value.Unwrap()
	}
	return result, nil
}

func semanticID(value strict_types.Value) (string, bool) {
	raw, ok := value.Bytes()
	if !ok || len(raw) != 32 {
		return "", false
	}
	return hex.EncodeToString(raw), true
}

func entries(value strict_types.Value) ([]strict_types.Entry, bool) {
	value = value.Unwrap()
	return value.Entries, value.Kind == strict_types.ValueMap
}

func sequence(value strict_types.Value) ([]strict_types.Value, bool) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueList && value.Kind != strict_types.ValueSet {
		return nil, false
	}
	return value.Items, true
}

func number(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return number(*value.Inner)
	}
	return value.Uint64()
}
