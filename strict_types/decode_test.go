package strict_types_test

import (
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	strict_types "github.com/sat20-labs/rgb11/strict_types"
)

func TestRC11RegistryDecodesOfficialTransfer(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/armored_transfer.txt")
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
	value, err := registry.Decode("RGBStd", "Consignmenttrue", armor.Data)
	if err != nil {
		t.Fatal(err)
	}
	transfer, ok := value.Field("transfer")
	if !ok {
		t.Fatal("decoded consignment has no transfer field")
	}
	isTransfer, ok := transfer.Bool()
	if !ok || !isTransfer {
		t.Fatalf("unexpected transfer flag: %+v", transfer)
	}
	bundles, ok := value.Field("bundles")
	if !ok || bundles.Kind != strict_types.ValueList || len(bundles.Items) != 0 {
		t.Fatalf("unexpected default transfer bundles: %+v", bundles)
	}
}

func TestRC11RegistryRejectsTrailingData(t *testing.T) {
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Decode("RGBStd", "ContainerVer", []byte{0, 1}); err == nil {
		t.Fatal("expected trailing data error")
	}
}

func TestRC11RegistryDecodesOfficialNIAContract(t *testing.T) {
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
	value, err := registry.Decode("RGBStd", "Consignmentfalse", armor.Data)
	if err != nil {
		t.Fatal(err)
	}
	transfer, ok := value.Field("transfer")
	if !ok {
		t.Fatal("decoded contract has no transfer field")
	}
	isTransfer, ok := transfer.Bool()
	if !ok || isTransfer {
		t.Fatalf("unexpected transfer flag: %+v", transfer)
	}
	genesis, ok := value.Field("genesis")
	if !ok {
		t.Fatal("decoded contract has no genesis")
	}
	timestamp, ok := genesis.Field("timestamp")
	if !ok || timestamp.Signed == nil || *timestamp.Signed != 1713261744 {
		t.Fatalf("unexpected genesis timestamp: %+v", timestamp)
	}
}
