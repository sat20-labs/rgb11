package schemas_test

import (
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/schemas"
)

func TestExtractOfficialNIAMetadata(t *testing.T) {
	armored, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := consignment.DecodeArmor(string(armored))
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := container.Value.Field("schema")
	types, _ := container.Value.Field("types")
	genesis, _ := container.Value.Field("genesis")
	metadata, err := schemas.ExtractGenesisAssetMetadata(schema, types, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Ticker == "" || metadata.DisplayName == "" || metadata.Precision > 18 ||
		metadata.IssuedSupply != 100_000 || metadata.MaxSupply != 100_000 {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func TestExtractEveryOfficialWalletSchemaMetadata(t *testing.T) {
	for _, fixture := range []string{
		"nia-example.rgba",
		"ifa-example.rgba",
		"cfa-example.rgba",
		"uda-example.rgba",
	} {
		t.Run(fixture, func(t *testing.T) {
			armored, err := os.ReadFile("../testvectors/rc11/" + fixture)
			if err != nil {
				t.Fatal(err)
			}
			container, err := consignment.DecodeArmor(string(armored))
			if err != nil {
				t.Fatal(err)
			}
			schema, _ := container.Value.Field("schema")
			types, _ := container.Value.Field("types")
			genesis, _ := container.Value.Field("genesis")
			metadata, err := schemas.ExtractGenesisAssetMetadata(schema, types, genesis)
			if err != nil {
				t.Fatal(err)
			}
			if metadata.DisplayName == "" || metadata.Precision > 18 {
				t.Fatalf("unexpected metadata: %+v", metadata)
			}
		})
	}
}
