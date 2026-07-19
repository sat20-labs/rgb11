package consignment

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

func decodeOfficialValue(t *testing.T, path, typeName string) strict_types.Value {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	armor, err := ParseArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	value, err := registry.Decode("RGBStd", typeName, armor.Data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestOfficialGenesisDisclosure(t *testing.T) {
	value := decodeOfficialValue(t, "../testvectors/rc11/nia-example.rgba", "Consignmentfalse")
	genesis, ok := value.Field("genesis")
	if !ok {
		t.Fatal("official consignment has no genesis")
	}
	actual, err := operations.DiscloseHash(genesis)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "c79f4b7eacbddc3dadc7b89cddb845b1ccf13e7f61e8e743dae7f50f3a43dac8"
	if hex.EncodeToString(actual[:]) != expected {
		encoded, encodeErr := operations.EncodeDisclose(genesis)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		assignments, _ := genesis.Field("assignments")
		first := assignments.Unwrap().Entries[0].Value.Unwrap().Inner.Unwrap().Items[0].Unwrap().Inner.Unwrap()
		seal, _ := first.Field("seal")
		t.Fatalf("genesis disclosure mismatch: got %x want %s; encoded=%x; seal=%#v", actual, expected, encoded, seal)
	}
}

func TestOfficialConsignmentIDComponents(t *testing.T) {
	value := decodeOfficialValue(t, "../testvectors/rc11/nia-example.rgba", "Consignmentfalse")
	types, _ := value.Field("types")
	typesID, err := typeSystemID(types)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(typesID[:]), "bc921cba60d9008af4c910948d9f1238f80f474bbe2562a3e8601aa119607b71"; got != want {
		t.Errorf("type system id = %s, want %s", got, want)
	}
	scripts, _ := value.Field("scripts")
	scriptIDs, err := aluVMLibraryIDs(scripts)
	if err != nil {
		t.Fatalf("%v: scripts=%#v", err, scripts)
	}
	if got, want := hex.EncodeToString(scriptIDs[0][:]), "abf099d28bed50df5e065715327f3a9b329f777cb0b9fefff634c193a03cb626"; got != want {
		t.Errorf("script id = %s, want %s", got, want)
	}
	consignmentID, err := ID(value)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(consignmentID[:]), "d79c62b85288aa9dc16021a642d3e95d71a13ed09cebb654e2847f3e016b8abb"; got != want {
		t.Errorf("consignment id = %s, want %s", got, want)
	}
}

func TestOfficialConsignmentIDs(t *testing.T) {
	for _, path := range []string{"../testvectors/rc11/nia-example.rgba", "../testvectors/rc11/nia-transfer.rgba"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeArmor(string(raw)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestOfficialTransitionFieldEncodings(t *testing.T) {
	value := decodeOfficialValue(t, "../testvectors/rc11/nia-transfer.rgba", "Consignmenttrue")
	bundles, _ := value.Field("bundles")
	bundle := bundles.Unwrap().Items[0].Unwrap()
	transitionBundle, _ := bundle.Field("bundle")
	known, _ := transitionBundle.Unwrap().Field("knownTransitions")
	knownTransition := known.Unwrap().Items[0].Unwrap()
	transition, _ := knownTransition.Field("transition")
	for _, name := range []string{"ffv", "contractId", "nonce", "transitionType", "metadata", "globals", "inputs", "assignments", "signature"} {
		field, ok := transition.Field(name)
		if !ok || len(field.Encoded) == 0 && name != "metadata" && name != "globals" {
			t.Fatalf("missing transition field %s", name)
		}
		t.Logf("%s=%x", name, field.Encoded)
	}
}
