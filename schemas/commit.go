package schemas

import (
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/schema/schema.rs
// Upstream-File-SHA256: f91e2b45fdd9afc8081f1e5ef72cebe05d472493562fd270f591c75c3242e92f
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
