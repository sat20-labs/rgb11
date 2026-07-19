package strict_types_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestOfficialConsignmentsStrictEncodeRoundTrip(t *testing.T) {
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	for _, vector := range []struct {
		path     string
		typeName string
	}{
		{"../testvectors/rc11/nia-example.rgba", "Consignmentfalse"},
		{"../testvectors/rc11/nia-transfer.rgba", "Consignmenttrue"},
	} {
		raw, err := os.ReadFile(vector.path)
		if err != nil {
			t.Fatal(err)
		}
		container, err := consignment.DecodeArmor(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := registry.Encode("RGBStd", vector.typeName, container.Value)
		if err != nil {
			t.Fatalf("encode %s: %v", vector.path, err)
		}
		if !bytes.Equal(encoded, container.Armor.Data) {
			t.Fatalf("strict round trip mismatch for %s", vector.path)
		}
	}
}
