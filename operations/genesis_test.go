package operations_test

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestOfficialNIAGenesisCommitment(t *testing.T) {
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
	genesis, ok := container.Field("genesis")
	if !ok {
		t.Fatal("genesis missing")
	}
	commitment, err := operations.CommitGenesis(genesis)
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"issuer":      "32fc587774e1496a1162b7eb0abb8e5c825fcd232da4678777dc49eef65485a1",
		"metadata":    "bd1380e8912f273087938a93b9ed8f497b5358187cb1b36d7f6f332015dd5353",
		"globals":     "bf9873b9fe556cc9282e7a1eb22adc3f56d3383eaa95e26a9272ead4ba123277",
		"assignments": "71087d95c62ce3850a861cba7fed3de0c4efc8c97b8869295459ec6570bf7d64",
	}
	got := map[string][32]byte{
		"issuer": commitment.IssuerHash, "metadata": commitment.MetadataHash,
		"globals": commitment.GlobalsRoot, "assignments": commitment.AssignmentsRoot,
	}
	for name, want := range wants {
		value := got[name]
		if hex.EncodeToString(value[:]) != want {
			t.Fatalf("%s commitment %x, want %s", name, value, want)
		}
	}
	contractID, err := operations.GenesisContractID(genesis)
	if err != nil {
		t.Fatal(err)
	}
	if contractID != armor.Contract {
		t.Fatalf("contract id %s, want %s", contractID, armor.Contract)
	}
}
