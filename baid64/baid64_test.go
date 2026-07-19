package baid64

import (
	"errors"
	"testing"
)

func TestRGBContractRoundTrip(t *testing.T) {
	canonical := "rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E"
	payload, err := Decode32(canonical, RGBContractOptions())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Encode32(payload, RGBContractOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("canonical round trip %q != %q", got, canonical)
	}
}

func TestSecretSealVector(t *testing.T) {
	canonical := "utxob:xDfmDF9g-yNOjriV-6Anbe6H-MLJ__g6-lo7Dd4f-dhWBW8S-XYGBm"
	payload, err := Decode32(canonical, UTXOBlindOptions())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Encode32(payload, UTXOBlindOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("canonical round trip %q != %q", got, canonical)
	}
}

func TestDecodeRejectsUncheckedMnemonic(t *testing.T) {
	_, err := Decode32("rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E#wrong-checksum", RGBContractOptions())
	if !errors.Is(err, ErrInvalidMnemonic) {
		t.Fatalf("expected ErrInvalidMnemonic, got %v", err)
	}
}

func TestSchemaIDMnemonicVector(t *testing.T) {
	canonical := "rgb:sch:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA#distant-history-exotic"
	payload, err := Decode32(canonical, SchemaIDOptions())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Encode32(payload, SchemaIDOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("canonical round trip %q != %q", got, canonical)
	}
}
