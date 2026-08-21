package wallet

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/sat20-labs/rgb11/invoicing"
	"github.com/sat20-labs/rgb11/seals"
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
	if loaded.Invoice != request.Invoice || loaded.Seal.Blinding == 0 || loaded.RelayKey != "" || loaded.AckKey != "" {
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

func TestCreateStandardWitnessReceiveOmitsSAT20Extensions(t *testing.T) {
	store := storage.NewMemoryStore()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatal(err)
	}
	engine.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	xonly, _ := hex.DecodeString("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	script := append([]byte{0x51, 0x20}, xonly...)
	amount := uint64(100)
	transport, err := invoicing.ParseTransport("rpcs://proxy.example.com/0.2/json-rpc")
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.CreateReceive(ReceiveParams{
		Mode: ReceiveWitness, ContractID: "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts",
		SchemaID: "XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M", Network: invoicing.BitcoinTestnet4,
		Amount: &amount, AssignmentName: "assetOwner", RecipientID: "recipient-1",
		WitnessVout: 1, WitnessScript: script, Expiry: 1_800_003_600,
		Transports: []invoicing.Transport{transport}, StandardOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := invoicing.Parse(request.Invoice)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Transports) != 1 || parsed.Transports[0].String() != transport.String() {
		t.Fatalf("unexpected transports: %+v", parsed.Transports)
	}
	if len(parsed.UnknownQuery) != 0 {
		t.Fatalf("standard invoice leaked SAT20 query parameters: %+v", parsed.UnknownQuery)
	}
	if request.RelayKey != "" || request.AckKey != "" {
		t.Fatal("standard receive state retained legacy SAT20 relay keys")
	}
}

func TestReceiveAcknowledgedMayAdvanceToAccepted(t *testing.T) {
	store := storage.NewMemoryStore()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatal(err)
	}
	engine.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	request, err := engine.CreateReceive(ReceiveParams{
		Network: invoicing.BitcoinTestnet4, RecipientID: "recipient-1",
		WitnessVout: 1, Expiry: 1_800_003_600,
	})
	if err != nil {
		t.Fatal(err)
	}
	objectHash := string(bytes.Repeat([]byte{'a'}, 64))
	witnessTxID := string(bytes.Repeat([]byte{'b'}, 64))
	if err := engine.MarkRelayAcknowledged(
		request.RequestID, "transfer-1", objectHash, witnessTxID,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.LoadReceive(request.RequestID)
	if err != nil || loaded.Status != ReceiveAcknowledged || loaded.WitnessTxID != witnessTxID {
		t.Fatalf("unexpected acknowledged receive: %+v err=%v", loaded, err)
	}
	if err := engine.MarkRelayAccepted(request.RequestID, "transfer-1", objectHash); err != nil {
		t.Fatal(err)
	}
	loaded, err = engine.LoadReceive(request.RequestID)
	if err != nil || loaded.Status != ReceiveAccepted {
		t.Fatalf("unexpected accepted receive: %+v err=%v", loaded, err)
	}
}

func TestReceiveAcknowledgedMayBeRejected(t *testing.T) {
	store := storage.NewMemoryStore()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatal(err)
	}
	amount := uint64(1)
	request, err := engine.CreateReceive(ReceiveParams{
		Mode: ReceiveBlind, Network: invoicing.BitcoinTestnet4,
		Amount: &amount, RecipientID: "recipient",
		WitnessVout: 1, Expiry: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	objectHash := strings.Repeat("11", 32)
	witnessTxID := strings.Repeat("22", 32)
	if err := engine.MarkRelayAcknowledged(
		request.RequestID, "transfer", objectHash, witnessTxID,
	); err != nil {
		t.Fatal(err)
	}
	if err := engine.MarkRelayRejected(
		request.RequestID, "transfer", objectHash, "validation-failed",
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.LoadReceive(request.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != ReceiveFailed || loaded.FailureCode != "validation-failed" {
		t.Fatalf("unexpected rejected receive: %+v", loaded)
	}
}

func TestReceiveRequestStorageUsesStrictEncoding(t *testing.T) {
	request := &ReceiveRequest{
		Version: ReceiveVersion, Mode: ReceiveBlind, RequestID: "request-1", RecipientID: "recipient-1",
		Seal: seals.NewWitnessBlindSeal(2, 42), Invoice: "rgb:invoice",
		CreatedAt: 1_800_000_000, Expiry: 1_800_003_600, Status: ReceivePrepared,
	}
	encoded, err := EncodeReceiveRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(encoded, []byte(receiveStoreMagic)) {
		t.Fatalf("receive request is not strict-encoded")
	}
	restored, err := DecodeReceiveRequest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.RequestID != request.RequestID || restored.Seal != request.Seal || restored.Invoice != request.Invoice {
		t.Fatalf("strict round trip differs: %#v", restored)
	}
	if _, err := DecodeReceiveRequest([]byte(`{"request_id":"legacy"}`)); err == nil {
		t.Fatal("legacy JSON receive record was accepted")
	}
}
