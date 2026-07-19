package consignment

import (
	"os"
	"testing"
)

func TestMergeHistoriesDeduplicatesSharedBundle(t *testing.T) {
	text, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	first, err := DecodeArmor(string(text))
	if err != nil {
		t.Fatal(err)
	}
	second, err := DecodeArmor(string(text))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeHistories(first, second)
	if err != nil {
		t.Fatal(err)
	}
	bundles, ok := merged.Value.Field("bundles")
	if !ok || len(bundles.Unwrap().Items) != 1 {
		t.Fatalf("merged bundle count = %d", len(bundles.Unwrap().Items))
	}
	if _, err := EncodeArmor(merged.Value); err != nil {
		t.Fatal(err)
	}
}

func TestMergeHistoriesRejectsDifferentContracts(t *testing.T) {
	transferText, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := DecodeArmor(string(transferText))
	if err != nil {
		t.Fatal(err)
	}
	other := *transfer
	other.ContractID = "rgb:different"
	if _, err := MergeHistories(transfer, &other); err == nil {
		t.Fatal("different contracts were merged")
	}
}
