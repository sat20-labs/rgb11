package schemas_test

import (
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestOfficialNIASchemaCommitment(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	armor, err := consignment.ParseArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	container, err := registry.Decode("RGBStd", "Consignmentfalse", armor.Data)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := container.Field("schema")
	if !ok {
		t.Fatal("schema missing")
	}
	id, err := schemas.ID(schema)
	if err != nil {
		t.Fatal(err)
	}
	if id != armor.Schema {
		t.Fatalf("schema id %s, want %s", id, armor.Schema)
	}
}
