package operations

import (
	"encoding/hex"
	"testing"

	"github.com/sat20-labs/rgb11/strict_types"
)

func TestOfficialNiaTransitionCommitment(t *testing.T) {
	const encodedHex = "0000934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec2a00000000000000102700000100934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936eca00f00000100a00f010100000001000000080706050403020108a08601000000000000"
	const wantID = "1e986e8714b4d3be6835797190a218be42831629d516cc26ebcf329e25716ad1"
	encoded, err := hex.DecodeString(encodedHex)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	transition, err := registry.Decode("RGBCommit", "Transition", encoded)
	if err != nil {
		t.Fatal(err)
	}
	commitment, err := CommitTransition(transition)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(commitment.OperationID[:]); got != wantID {
		t.Fatalf("transition id = %s, want %s", got, wantID)
	}
}
