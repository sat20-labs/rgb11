package anchors

import (
	"encoding/hex"
	"testing"
)

func TestOpretScript(t *testing.T) {
	commitment := [32]byte{1, 2, 3}
	script := OpretScript(commitment)
	if err := VerifyOpretScript(script, commitment); err != nil {
		t.Fatal(err)
	}
}

func TestTapretRootPathRustVector(t *testing.T) {
	internalBytes, _ := hex.DecodeString("c5f93479093e2b8f724a79844cc10928dd44e9a390b539843fb83fbf842723f3")
	var internal, commitment [32]byte
	copy(internal[:], internalBytes)
	for index := range commitment {
		commitment[index] = 8
	}
	output, err := TapretOutputKey(internal, commitment, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(output[:]), "01dec901755c1e91f17dfb3a0ef43be679c85b27a81414034255c6d1dff69278"; got != want {
		t.Fatalf("tapret output key = %s, want %s", got, want)
	}
	if script := TapretScript(commitment, 0); len(script) != 64 || script[29] != 0x6a || script[30] != 0x21 {
		t.Fatalf("invalid tapret script %x", script)
	}
}
