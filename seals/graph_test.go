package seals

import (
	"encoding/hex"
	"testing"
)

func TestWitnessGraphSealRustVector(t *testing.T) {
	seal := NewWitnessBlindSeal(7, 0x0102030405060708)
	encoded, err := seal.StrictBytes()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(encoded), "00070000000807060504030201"; got != want {
		t.Fatalf("strict graph seal = %s, want %s", got, want)
	}
	concealed, err := seal.Conceal()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := concealed.String(), "utxob:IIEuw5Fu-LBXIPwE-3e7Q6jy-bmNpKrk-1REyI42-ImMiOgp-rY2M0"; got != want {
		t.Fatalf("concealed = %s, want %s", got, want)
	}
}

func TestExplicitGraphSealRustVector(t *testing.T) {
	raw, err := hex.DecodeString("01c568200c10c4ca3c351108bffc3d1e4238f94d94c06d28b6cd91a1b15b5d291401000000a186010000000000")
	if err != nil {
		t.Fatal(err)
	}
	seal, err := DecodeGraphBlindSeal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if seal.TxID == nil || seal.Vout != 1 || seal.Blinding != 100001 {
		t.Fatalf("decoded explicit graph seal = %+v", seal)
	}
	encoded, err := seal.StrictBytes()
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(encoded); got != hex.EncodeToString(raw) {
		t.Fatalf("explicit graph seal strict encoding = %s", got)
	}
	concealed, err := seal.Conceal()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := concealed.String(), "utxob:IUUtEi1r-zGc68Nw-82LNcuE-32RQ1Hc-GLxeJHU-om8i2Xc-e3t7L"; got != want {
		t.Fatalf("explicit graph seal concealed = %s, want %s", got, want)
	}
}
