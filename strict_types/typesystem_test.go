package strict_types_test

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestContractTypeSystemDecodesAllNiaGlobals(t *testing.T) {
	armored, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := consignment.DecodeArmor(string(armored))
	if err != nil {
		t.Fatal(err)
	}
	types, ok := container.Value.Field("types")
	if !ok {
		t.Fatal("consignment types missing")
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddTypeSystem("contract", types); err != nil {
		t.Fatal(err)
	}

	schema, _ := container.Value.Field("schema")
	genesis, _ := container.Value.Field("genesis")
	globalTypes, _ := schema.Field("globalTypes")
	globals, _ := genesis.Field("globals")
	semantics := map[uint64]string{}
	for _, entry := range globalTypes.Unwrap().Entries {
		typeID, ok := entry.Key.Unwrap().Uint64()
		if !ok {
			t.Fatal("invalid global type key")
		}
		details := entry.Value.Unwrap()
		stateSchema, ok := details.Field("globalStateSchema")
		if !ok {
			t.Fatal("global state schema missing")
		}
		semID, ok := stateSchema.Unwrap().Field("semId")
		if !ok {
			t.Fatal("global semantic id missing")
		}
		raw, ok := semID.Bytes()
		if !ok || len(raw) != 32 {
			t.Fatal("invalid global semantic id")
		}
		semantics[typeID] = hexString(raw)
	}
	decoded := 0
	for _, entry := range globals.Unwrap().Entries {
		typeID, ok := entry.Key.Unwrap().Uint64()
		if !ok {
			t.Fatal("invalid global key")
		}
		semID := semantics[typeID]
		if semID == "" {
			t.Fatalf("schema lacks global type %d", typeID)
		}
		values := entry.Value.Unwrap()
		for _, state := range values.Items {
			raw, ok := state.Bytes()
			if !ok {
				t.Fatal("invalid revealed global state")
			}
			decodedValue, err := registry.DecodeSemantic("contract", semID, raw)
			if err != nil {
				definition, _ := registry.SemanticTypeDefinition("contract", semID)
				textDef, _ := registry.SemanticTypeDefinition("contract", "18cb946f1293cf180e9d78dcc65bc59b472ffffeadfbf58db198cc8328f64b01")
				mediaDef, _ := registry.SemanticTypeDefinition("contract", "e087a83496338799afc48a9211683a427d2bd33e2ea7ebb8a8b880ea4ab4eb81")
				textInner, _ := registry.SemanticTypeDefinition("contract", "560d96f7a47924b2c3df040e6463398fd65fd591652c294342bfa5f939155154")
				noneDef, _ := registry.SemanticTypeDefinition("contract", "d83fbee02f0de5b46cf80fe11ef7fdf061c78d975d31ade9eea2bc4099339e6c")
				t.Fatalf("decode global type %d sem=%s raw=%s def=%s text=%s textInner=%s media=%s none=%s: %v", typeID, semID, hex.EncodeToString(raw), definition, textDef, textInner, mediaDef, noneDef, err)
			}
			reencoded, err := registry.EncodeSemantic("contract", semID, decodedValue)
			if err != nil {
				t.Fatalf("encode global type %d: %v", typeID, err)
			}
			if hex.EncodeToString(reencoded) != hex.EncodeToString(raw) {
				t.Fatalf("global type %d semantic roundtrip mismatch", typeID)
			}
			decoded++
		}
	}
	if decoded != 3 {
		t.Fatalf("decoded %d global states, want 3", decoded)
	}
}

func hexString(data []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(data)*2)
	for i, value := range data {
		result[i*2] = alphabet[value>>4]
		result[i*2+1] = alphabet[value&15]
	}
	return string(result)
}
