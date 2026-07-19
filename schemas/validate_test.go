package schemas_test

import (
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestValidateOfficialNiaGenesis(t *testing.T) {
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
	report, err := schemas.ValidateGenesis(schema, types, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != schemas.NIA || !report.ConformanceValid || !report.StateTypesValid || !report.ScriptValid {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.GlobalStates != 3 || report.Assignments != 1 || report.FungibleAmounts[4000] != 100000 {
		t.Fatalf("unexpected state summary: %+v", report)
	}
}

func TestValidateEveryOfficialWalletSchemaGenesis(t *testing.T) {
	fixtures := map[string]schemas.Kind{
		"nia-example.rgba": schemas.NIA,
		"ifa-example.rgba": schemas.IFA,
		"cfa-example.rgba": schemas.CFA,
		"uda-example.rgba": schemas.UDA,
	}
	for fixture, kind := range fixtures {
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
			report, err := schemas.ValidateGenesis(schema, types, genesis)
			if err != nil {
				t.Fatal(err)
			}
			if report.Kind != kind || !report.ConformanceValid || !report.StateTypesValid || !report.ScriptValid {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

func TestNiaGenesisRejectsForgedIssuedSupply(t *testing.T) {
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
	globals, _ := genesis.Field("globals")
	globalMap := globals.Unwrap()
	for index := range globalMap.Entries {
		entry := &globalMap.Entries[index]
		id, _ := testNumber(entry.Key)
		if id == 2010 {
			states := entry.Value.Unwrap()
			if !mutateFirstByte(&states.Items[0]) {
				t.Fatal("issued supply payload not found")
			}
		}
	}
	if _, err := schemas.ValidateGenesis(schema, types, genesis); err != nil {
		return
	}
	t.Fatal("forged issued/allocation state accepted")
}

func mutateFirstByte(value *strict_types.Value) bool {
	if value.Kind == strict_types.ValueBytes && len(value.Raw) > 0 {
		value.Raw[0]++
		return true
	}
	if value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		return mutateFirstByte(&value.Items[0])
	}
	return false
}

func testNumber(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return testNumber(*value.Inner)
	}
	return value.Uint64()
}
