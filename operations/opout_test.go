package operations

import (
	"encoding/hex"
	"testing"
)

func TestOpoutRoundTripAndStrictEncoding(t *testing.T) {
	value := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f/4000/7"
	opout, err := ParseOpout(value)
	if err != nil {
		t.Fatal(err)
	}
	if opout.String() != value {
		t.Fatalf("opout = %s", opout)
	}
	encoded, err := opout.StrictBytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" + "a00f0700"
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("strict opout = %s, want %s", got, want)
	}
}
