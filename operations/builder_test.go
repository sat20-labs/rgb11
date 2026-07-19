package operations_test

import (
	"encoding/hex"
	"testing"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/seals"
)

func TestBuildConfidentialTransitionMatchesRust(t *testing.T) {
	contract := decodeBuilder32(t, "934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec")
	seal, err := seals.NewWitnessBlindSeal(1, 0x0102030405060708).Conceal()
	if err != nil {
		t.Fatal(err)
	}
	value, commitment, err := operations.BuildTransition(operations.TransitionSpec{
		ContractID: contract, Nonce: 42, TransitionType: 10000,
		Inputs:  []operations.TransitionInput{{OperationID: contract, AssignmentType: 4000}},
		Outputs: []operations.TransitionOutput{{AssignmentType: 4000, Class: "fungible", Amount: 100_000, SecretSeal: [32]byte(seal)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const expected = "0000934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec2a00000000000000102700000100934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936eca00f00000100a00f010100017562564794a95ffaafc9f3b1520bc41f7ed1cf5bd28a6c859ac29def72dd497508a08601000000000000"
	if got := hex.EncodeToString(value.Encoded); got != expected {
		t.Fatalf("transition strict encoding = %s, want %s", got, expected)
	}
	if got, want := hex.EncodeToString(commitment.OperationID[:]), "1e986e8714b4d3be6835797190a218be42831629d516cc26ebcf329e25716ad1"; got != want {
		t.Fatalf("transition id = %s, want %s", got, want)
	}
}

func TestBuildRevealedTransitionMatchesRust(t *testing.T) {
	contract := decodeBuilder32(t, "934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec")
	seal := seals.NewWitnessBlindSeal(1, 0x0102030405060708)
	value, commitment, err := operations.BuildTransition(operations.TransitionSpec{
		ContractID: contract, Nonce: 42, TransitionType: 10000,
		Inputs:  []operations.TransitionInput{{OperationID: contract, AssignmentType: 4000}},
		Outputs: []operations.TransitionOutput{{AssignmentType: 4000, Class: "fungible", Amount: 100_000, RevealedSeal: &seal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const expected = "0000934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec2a00000000000000102700000100934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936eca00f00000100a00f010100000001000000080706050403020108a08601000000000000"
	if got := hex.EncodeToString(value.Encoded); got != expected {
		t.Fatalf("transition strict encoding = %s, want %s", got, expected)
	}
	if got, want := hex.EncodeToString(commitment.OperationID[:]), "1e986e8714b4d3be6835797190a218be42831629d516cc26ebcf329e25716ad1"; got != want {
		t.Fatalf("transition id = %s, want %s", got, want)
	}
}

func decodeBuilder32(t *testing.T, value string) [32]byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 {
		t.Fatal(err)
	}
	var out [32]byte
	copy(out[:], raw)
	return out
}
