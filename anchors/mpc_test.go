package anchors_test

import (
	"encoding/hex"
	"testing"

	"github.com/sat20-labs/rgb11/anchors"
)

func TestOfficialNIAMPCProof(t *testing.T) {
	protocol := decode32(t, "934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec")
	message := decode32(t, "3696f9eeba53cd9727f604e690c4b7f1b0f2b2be239276b903e6ae385855013e")
	proof := anchors.MPCProof{Position: 3, Path: [][32]byte{
		decode32(t, "739e0243627f14703277a68b11bbd1e30acccead5b30c837a574993d6aba5b25"),
		decode32(t, "2433a1c7b5932426404c97abf7cebe859821bf9f69df2f91eb242c9a998ab143"),
		decode32(t, "bca7443784a3148ecce21fc04c41ee2da9f981c0b7a91109eac8ea55c77e4507"),
	}}
	commitment, err := anchors.ConvolveMPC(proof, protocol, message)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(commitment[:]); got != "8bef6db012dbd42088e5af8ac1df536ff8de140e82fe34a0bbb3e13b912b55b3" {
		t.Fatalf("MPC commitment mismatch: %s", got)
	}
}

func decode32(t *testing.T, text string) [32]byte {
	t.Helper()
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != 32 {
		t.Fatalf("decode 32-byte value %q: %v", text, err)
	}
	var value [32]byte
	copy(value[:], raw)
	return value
}
