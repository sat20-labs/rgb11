package consignment

import (
	"bytes"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/strict_types"
)

func TestBuildOfficialTransferConsignment(t *testing.T) {
	contractRaw, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DecodeArmor(string(contractRaw))
	if err != nil {
		t.Fatal(err)
	}
	transferRaw, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := DecodeArmor(string(transferRaw))
	if err != nil {
		t.Fatal(err)
	}
	bundles, _ := transfer.Value.Field("bundles")
	witnessBundle := bundles.Unwrap().Items[0]
	terminals, _ := transfer.Value.Field("terminals")
	secrets := make([][32]byte, 0)
	for _, terminal := range terminals.Unwrap().Entries {
		secretSet := terminal.Value.Unwrap()
		for _, item := range secretSet.Items {
			raw, ok := item.Bytes()
			if !ok || len(raw) != 32 {
				t.Fatal("invalid official terminal secret")
			}
			var secret [32]byte
			copy(secret[:], raw)
			secrets = append(secrets, secret)
		}
	}
	built, err := BuildTransfer(contract, witnessBundle, secrets)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := registry.Encode("RGBStd", "Consignmenttrue", built.Value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, transfer.Armor.Data) {
		t.Fatal("built transfer differs from official strict payload")
	}
	armored, err := EncodeArmor(built.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeArmor(armored); err != nil {
		t.Fatalf("built transfer armor is not self-validating: %v", err)
	}
}
