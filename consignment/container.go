package consignment

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-ops
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 5308b9d46c91857513ff5be2459992264687632b
// Upstream-File: src/containers/consignment.rs
// Translation-Revision: 1

var (
	ErrContainerType    = errors.New("RGB11 consignment container type mismatch")
	ErrContainerVersion = errors.New("unsupported RGB11 consignment container version")
	ErrContractMismatch = errors.New("RGB11 consignment contract id mismatch")
	ErrSchemaMismatch   = errors.New("RGB11 consignment schema id mismatch")
	ErrGenesisSchema    = errors.New("RGB11 genesis references a different schema")
	ErrFileMagic        = errors.New("invalid RGB11 container file magic")
)

// Container is a strictly decoded RGB11 contract or transfer. DecodeArmor
// validates transport integrity, strict confinement, schema commitment and
// genesis contract commitment. It intentionally does not set ConsensusValid:
// bundle anchors, witness history and schema VM execution are validated by the
// consensus validator built on top of this representation.
type Container struct {
	Armor           *Armor
	Value           strict_types.Value
	ContractID      string
	SchemaID        string
	StructuralValid bool
	GenesisValid    bool
	GenesisReport   schemas.GenesisValidation
	ConsensusValid  bool
}

// Decode accepts either the official ASCII armor transport or the canonical
// strict-encoded container bytes written by rgb-lib. Binary inputs are decoded
// first and then passed through the same commitment and schema checks as armor.
func Decode(raw []byte) (*Container, error) {
	if bytes.Contains(raw, []byte(armorBegin)) {
		return DecodeArmor(string(raw))
	}
	payload := raw
	typeNames := []string{"Consignmenttrue", "Consignmentfalse"}
	if len(raw) >= 7 && bytes.Equal(raw[:4], []byte{'R', 'G', 'B', 0}) {
		switch string(raw[4:7]) {
		case "TFR":
			typeNames = []string{"Consignmenttrue"}
		case "CON":
			typeNames = []string{"Consignmentfalse"}
		default:
			return nil, ErrFileMagic
		}
		payload = raw[7:]
	}
	if len(payload) == 0 || len(payload) > maxArmoredDataSize {
		return nil, ErrArmorTooLarge
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return nil, err
	}
	var value strict_types.Value
	var decodeErr error
	for _, typeName := range typeNames {
		value, decodeErr = registry.Decode("RGBStd", typeName, payload)
		if decodeErr == nil {
			armored, err := EncodeArmor(value)
			if err != nil {
				return nil, err
			}
			return DecodeArmor(armored)
		}
	}
	return nil, fmt.Errorf("strict decode RGB11 consignment: %w", decodeErr)
}

func DecodeArmor(text string) (*Container, error) {
	armor, err := ParseArmor(text)
	if err != nil {
		return nil, err
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return nil, err
	}
	typeName := "Consignmentfalse"
	wantTransfer := false
	if armor.Type == "transfer" {
		typeName = "Consignmenttrue"
		wantTransfer = true
	}
	value, err := registry.Decode("RGBStd", typeName, armor.Data)
	if err != nil {
		return nil, fmt.Errorf("strict decode RGB11 consignment: %w", err)
	}
	if err := verifyID(armor.ID, value); err != nil {
		return nil, err
	}
	version, ok := value.Field("version")
	if !ok || version.Unwrap().Kind != strict_types.ValueEnum || version.Unwrap().Tag != 0 {
		return nil, ErrContainerVersion
	}
	transfer, ok := value.Field("transfer")
	if !ok {
		return nil, ErrContainerType
	}
	isTransfer, ok := transfer.Bool()
	if !ok || isTransfer != wantTransfer {
		return nil, ErrContainerType
	}
	schema, ok := value.Field("schema")
	if !ok {
		return nil, ErrSchemaMismatch
	}
	schemaCommitment, err := schemas.Commitment(schema)
	if err != nil {
		return nil, err
	}
	schemaID, err := schemas.ID(schema)
	if err != nil {
		return nil, err
	}
	if armor.Schema != schemaID {
		return nil, fmt.Errorf("%w: header=%s computed=%s", ErrSchemaMismatch, armor.Schema, schemaID)
	}
	genesis, ok := value.Field("genesis")
	if !ok {
		return nil, ErrContractMismatch
	}
	genesisSchema, ok := genesis.Field("schemaId")
	if !ok {
		return nil, ErrGenesisSchema
	}
	genesisSchemaBytes, ok := genesisSchema.Bytes()
	if !ok || !bytes.Equal(genesisSchemaBytes, schemaCommitment[:]) {
		return nil, ErrGenesisSchema
	}
	contractID, err := operations.GenesisContractID(genesis)
	if err != nil {
		return nil, err
	}
	if armor.Contract != contractID {
		return nil, fmt.Errorf("%w: header=%s computed=%s", ErrContractMismatch, armor.Contract, contractID)
	}
	types, ok := value.Field("types")
	if !ok {
		return nil, fmt.Errorf("%w: contract type system missing", schemas.ErrStateType)
	}
	genesisReport, err := schemas.ValidateGenesis(schema, types, genesis)
	if err != nil {
		return nil, err
	}
	return &Container{
		Armor: armor, Value: value, ContractID: contractID, SchemaID: schemaID,
		StructuralValid: true, GenesisValid: true, GenesisReport: genesisReport,
	}, nil
}
