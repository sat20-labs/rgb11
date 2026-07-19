package consignment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

const armorColumns = 80

// EncodeArmor serializes a canonical RGBStd consignment and wraps it in the
// ASCII armor format used by the frozen Rust RGB 0.11.1-rc.11 stack.
func EncodeArmor(value strict_types.Value) (string, error) {
	transferValue, ok := value.Field("transfer")
	if !ok {
		return "", ErrContainerType
	}
	isTransfer, ok := transferValue.Bool()
	if !ok {
		return "", ErrContainerType
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return "", err
	}
	typeName := "Consignmentfalse"
	typeLabel := "contract"
	if isTransfer {
		typeName = "Consignmenttrue"
		typeLabel = "transfer"
	}
	data, err := registry.Encode("RGBStd", typeName, value)
	if err != nil {
		return "", fmt.Errorf("strict encode RGB11 consignment: %w", err)
	}
	if len(data) > maxArmoredDataSize {
		return "", ErrArmorTooLarge
	}
	// Re-decode before computing commitments. Builders may mutate a decoded
	// template, whose Value.Encoded caches still describe the template. IDs
	// must commit to the canonical bytes just produced, never stale caches.
	rebuilt, err := registry.Decode("RGBStd", typeName, data)
	if err != nil {
		return "", err
	}
	versionValue, ok := rebuilt.Field("version")
	if !ok {
		return "", ErrContainerVersion
	}
	versionValue = versionValue.Unwrap()
	if versionValue.Kind != strict_types.ValueEnum {
		return "", ErrContainerVersion
	}
	genesis, ok := rebuilt.Field("genesis")
	if !ok {
		return "", ErrContractMismatch
	}
	contractID, err := operations.GenesisContractID(genesis)
	if err != nil {
		return "", err
	}
	schema, ok := rebuilt.Field("schema")
	if !ok {
		return "", ErrSchemaMismatch
	}
	schemaID, err := schemas.ID(schema)
	if err != nil {
		return "", err
	}
	commitment, err := ID(rebuilt)
	if err != nil {
		return "", err
	}
	id, err := baid64.Encode32(commitment, baid64.ConsignmentIDOptions())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	body := encodeBase85(data)

	var text strings.Builder
	fmt.Fprintf(&text, "%s\n", armorBegin)
	fmt.Fprintf(&text, "Id: %s\n", id)
	fmt.Fprintf(&text, "Version: %d\n", versionValue.Tag)
	fmt.Fprintf(&text, "Type: %s\n", typeLabel)
	fmt.Fprintf(&text, "Contract: %s\n", contractID)
	fmt.Fprintf(&text, "Schema: %s\n", schemaID)
	fmt.Fprintf(&text, "Check-SHA256: %s\n\n", hex.EncodeToString(digest[:]))
	for len(body) > armorColumns {
		fmt.Fprintf(&text, "%s\n", body[:armorColumns])
		body = body[armorColumns:]
	}
	if body != "" {
		fmt.Fprintf(&text, "%s\n", body)
	}
	fmt.Fprintf(&text, "\n%s\n", armorEnd)
	return text.String(), nil
}
