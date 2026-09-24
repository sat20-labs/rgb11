package schemas

import (
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1
// Upstream-Commit: 32a67862aef0f4c7a1fdc7834a3555d702f1bdf0
// Upstream-File: src/schema/schema.rs
// Upstream-File-SHA256: 74013842290fedbc72ba051fe71877a03e24d41b16a8b0175c59bbad2e7f13e0
// Translation-Revision: 1

const SchemaCommitmentTag = "urn:lnp-bp:rgb:schema#2024-02-03"

var ErrInvalidSchemaValue = errors.New("invalid decoded RGB11 schema")

func Commitment(value strict_types.Value) ([32]byte, error) {
	if value.Kind != strict_types.ValueStruct {
		return [32]byte{}, ErrInvalidSchemaValue
	}
	fieldNames := [...]string{
		"ffv", "name", "metaTypes", "globalTypes", "ownedTypes", "genesis", "transitions", "defaultAssignment",
	}
	chunks := make([][]byte, 0, len(fieldNames))
	for _, name := range fieldNames {
		field, ok := value.Field(name)
		if !ok || field.Encoded == nil {
			return [32]byte{}, fmt.Errorf("%w: missing %s", ErrInvalidSchemaValue, name)
		}
		chunks = append(chunks, field.Encoded)
	}
	return consensus.TaggedHash(SchemaCommitmentTag, chunks...), nil
}

func ID(value strict_types.Value) (string, error) {
	commitment, err := Commitment(value)
	if err != nil {
		return "", err
	}
	return baid64.Encode32(commitment, baid64.SchemaIDOptions())
}
