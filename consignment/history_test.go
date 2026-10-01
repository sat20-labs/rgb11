package consignment

import (
	"errors"
	"testing"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

// Official 0.11.1 ResolveWitness checks history without an outspend API.
// Projection remains a separate, stricter wallet operation.
func TestValidateHistorySeparatesSpentTerminalFromProjection(t *testing.T) {
	c, resolver := loadValidationVector(t)
	resolver.currentSpent = true
	if err := c.ValidateHistory(resolver); err != nil {
		t.Fatal(err)
	}
	if c.ConsensusValid {
		t.Fatal("history-only check marked the container projection-valid")
	}
	if _, err := c.Validate(resolver); !errors.Is(err, ErrOutpointSpend) {
		t.Fatalf("spent receiving terminal accepted: %v", err)
	}
}

func TestValidateHistoryWitnessBoundaries(t *testing.T) {
	for _, kind := range []string{"archived", "unknown", "wrong_raw", "missing_raw"} {
		t.Run(kind, func(t *testing.T) {
			c, resolver := loadValidationVector(t)
			switch kind {
			case "archived":
				resolver.archived = true
			case "unknown":
				resolver.unknownWitness = true
			case "wrong_raw":
				resolver.raw = append([]byte(nil), resolver.raw...)
				resolver.raw[len(resolver.raw)-1] ^= 1
			case "missing_raw":
				resolver.raw = nil
			}
			if err := c.ValidateHistory(resolver); err == nil {
				t.Fatal("invalid historical witness accepted")
			}
		})
	}
}

func TestValidateHistoryRejectsDuplicateStateSpend(t *testing.T) {
	c, resolver := loadValidationVector(t)
	bundles := unwrapPointer(fieldPointer(&c.Value, "bundles"))
	bundles.Items = append(bundles.Items, bundles.Items[0])
	if err := c.ValidateHistory(resolver); !errors.Is(err, ErrDuplicateStateSpend) {
		t.Fatalf("duplicate historical state spend: %v", err)
	}
}

func TestValidateHistoryPreservesTerminalBinding(t *testing.T) {
	c, resolver := loadValidationVector(t)
	terminals := unwrapPointer(fieldPointer(&c.Value, "terminals"))
	terminals.Entries = []strict_types.Entry{{Key: strict_types.Value{Kind: strict_types.ValueBytes, Raw: make([]byte, 32)}}}
	if err := c.ValidateHistory(resolver); !errors.Is(err, operations.ErrInvalidBundle) {
		t.Fatalf("absent terminal bundle accepted as history: %v", err)
	}
}
