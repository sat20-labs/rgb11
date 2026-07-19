package wallet

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/sat20-labs/rgb11/invoicing"
	"github.com/sat20-labs/rgb11/storage"
)

func TestCreateReceivePersistsSealBeforeReturningInvoice(t *testing.T) {
	store := storage.NewMemoryStore()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatal(err)
	}
	engine.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	amount := uint64(100)
	request, err := engine.CreateReceive(ReceiveParams{
		ContractID: "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts",
		SchemaID:   "XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M",
		Network:    invoicing.BitcoinMainnet, Amount: &amount, AssignmentName: "assetOwner",
		RecipientID: "recipient-1", WitnessVout: 2, Expiry: 1_800_003_600,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := invoicing.Parse(request.Invoice)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Beneficiary.Kind != invoicing.BeneficiaryBlindedSeal || parsed.Beneficiary.Network != invoicing.BitcoinMainnet {
		t.Fatalf("unexpected beneficiary: %+v", parsed.Beneficiary)
	}
	loaded, err := engine.LoadReceive(request.RequestID)
	if err != nil {
		t.Fatalf("invoice returned without persisted seal: %v", err)
	}
	if loaded.Invoice != request.Invoice || loaded.Seal.Blinding == 0 || loaded.RelayKey == loaded.AckKey {
		t.Fatalf("incomplete persisted receive request: %+v", loaded)
	}
}

func TestCreateWitnessReceivePersistsScriptBeforeReturningInvoice(t *testing.T) {
	store := storage.NewMemoryStore()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatal(err)
	}
	engine.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	xonly, _ := hex.DecodeString("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	script := append([]byte{0x51, 0x20}, xonly...)
	amount := uint64(100)
	request, err := engine.CreateReceive(ReceiveParams{
		Mode: ReceiveWitness, ContractID: "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts",
		SchemaID: "XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M", Network: invoicing.BitcoinRegtest,
		Amount: &amount, AssignmentName: "assetOwner", RecipientID: "recipient-1",
		WitnessVout: 2, WitnessScript: script, Expiry: 1_800_003_600,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := invoicing.Parse(request.Invoice)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parsed.Beneficiary.WitnessScript()
	if err != nil || !bytes.Equal(got, script) {
		t.Fatalf("witness script = %x, %v; want %x", got, err, script)
	}
	loaded, err := engine.LoadReceive(request.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != ReceiveWitness || !bytes.Equal(loaded.WitnessScript, script) || loaded.Seal.Blinding != 0 {
		t.Fatalf("incomplete persisted witness receive: %+v", loaded)
	}
}
