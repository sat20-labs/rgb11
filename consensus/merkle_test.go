package consensus

import "testing"

func TestMerkleRootDeterminism(t *testing.T) {
	empty := MerkleRoot(nil)
	if empty == ([32]byte{}) {
		t.Fatal("empty Merkle root must commit to a void node")
	}
	one := MerkleLeafHash([]byte("one"))
	if got := MerkleRoot([][32]byte{one}); got != one {
		t.Fatalf("single-leaf root changed: %x", got)
	}
	two := MerkleLeafHash([]byte("two"))
	if MerkleRoot([][32]byte{one, two}) == one {
		t.Fatal("two-leaf root must create a branch node")
	}
}
