package schemas

import (
	"fmt"

	"github.com/sat20-labs/rgb11/strict_types"
)

// GenesisAssetMetadata is the display and denomination information committed
// by the standard-schema genesis. Amounts remain atomic u64 values; Precision
// only controls their decimal representation.
type GenesisAssetMetadata struct {
	Ticker        string
	DisplayName   string
	Precision     uint8
	IssuedSupply  uint64
	MaxSupply     uint64
	RejectListURL string
}

// ExtractGenesisAssetMetadata decodes the official AssetSpec/ContractSpec and
// supply globals using the TypeSystem carried by the consignment.
func ExtractGenesisAssetMetadata(schema, typeSystem, genesis strict_types.Value) (GenesisAssetMetadata, error) {
	descriptorID, err := ID(schema)
	if err != nil {
		return GenesisAssetMetadata{}, err
	}
	descriptor, err := ByID(descriptorID)
	if err != nil {
		return GenesisAssetMetadata{}, err
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return GenesisAssetMetadata{}, err
	}
	if err := registry.AddTypeSystem("contract", typeSystem); err != nil {
		return GenesisAssetMetadata{}, fmt.Errorf("%w: %v", ErrStateType, err)
	}
	genesisSchema, ok := schema.Field("genesis")
	if !ok {
		return GenesisAssetMetadata{}, fmt.Errorf("%w: genesis schema missing", ErrSchemaConformance)
	}
	globals, _, err := validateGlobals(registry, schema, genesisSchema, genesis)
	if err != nil {
		return GenesisAssetMetadata{}, err
	}

	var metadata GenesisAssetMetadata
	foundSpec := false
	if descriptor.Kind == CFA {
		const (
			globalName      = 3001
			globalPrecision = 3005
		)
		nameValues := globals[globalName]
		precisionValues := globals[globalPrecision]
		if len(nameValues) != 1 || len(precisionValues) != 1 {
			return GenesisAssetMetadata{}, fmt.Errorf("%w: CFA name or precision missing", ErrSchemaConformance)
		}
		name, ok := nameValues[0].decoded.TextValue()
		precision := precisionValues[0].decoded.Unwrap()
		if !ok || name == "" || precision.Kind != strict_types.ValueEnum || precision.Tag > 18 {
			return GenesisAssetMetadata{}, fmt.Errorf("%w: CFA specification", ErrStateType)
		}
		metadata.DisplayName = name
		metadata.Precision = precision.Tag
		foundSpec = true
	}
	for _, values := range globals {
		for _, item := range values {
			value := item.decoded.Unwrap()
			nameValue, hasName := value.Field("name")
			precisionValue, hasPrecision := value.Field("precision")
			if !hasName || !hasPrecision {
				continue
			}
			name, ok := nameValue.TextValue()
			precision := precisionValue.Unwrap()
			if !ok || name == "" || precision.Kind != strict_types.ValueEnum || precision.Tag > 18 {
				return GenesisAssetMetadata{}, fmt.Errorf("%w: asset specification", ErrStateType)
			}
			metadata.DisplayName = name
			metadata.Precision = precision.Tag
			if tickerValue, ok := value.Field("ticker"); ok {
				ticker, textOK := tickerValue.TextValue()
				if !textOK {
					return GenesisAssetMetadata{}, fmt.Errorf("%w: asset ticker", ErrStateType)
				}
				metadata.Ticker = ticker
			}
			foundSpec = true
		}
	}
	if !foundSpec {
		return GenesisAssetMetadata{}, fmt.Errorf("%w: asset specification missing", ErrSchemaConformance)
	}
	if issued, err := oneGlobalAmount(globals, globalIssuedSupply); err == nil {
		metadata.IssuedSupply = issued
		metadata.MaxSupply = issued
	}
	if maximum, err := oneGlobalAmount(globals, globalMaxSupply); err == nil {
		metadata.MaxSupply = maximum
	}
	if values := globals[globalRejectListURL]; len(values) > 0 {
		if len(values) != 1 {
			return GenesisAssetMetadata{}, fmt.Errorf("%w: reject list URL cardinality", ErrSchemaConformance)
		}
		url, ok := values[0].decoded.TextValue()
		if !ok {
			return GenesisAssetMetadata{}, fmt.Errorf("%w: reject list URL", ErrStateType)
		}
		metadata.RejectListURL = url
	}
	return metadata, nil
}
