package consensus

import "encoding/binary"

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1
// Upstream-Commit: 32a67862aef0f4c7a1fdc7834a3555d702f1bdf0
// Upstream-File: src/commit_verify/merkle.rs
// Upstream-File-SHA256: 0bb82332e989c6ae3c5bcaa5255efe6b63614a7bf047f38a693a4664b0af3b92
// Translation-Revision: 1

const (
	StrictValueCommitmentTag = "urn:ubideco:strict-types:value-hash#2024-02-10"
	MerkleNodeCommitmentTag  = "urn:ubideco:merkle:node#2024-01-31"
)

var virtualMerkleLeaf = [32]byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

func StrictValueHash(encoded []byte) [32]byte {
	return TaggedHash(StrictValueCommitmentTag, encoded)
}

func MerkleLeafHash(encoded []byte) [32]byte {
	return TaggedHash(MerkleNodeCommitmentTag, encoded)
}

// MerkleBranchHash exposes the LNPBP-81 branch commitment used by both RGB
// operation commitments and LNPBP-4 multi-protocol proofs. Width is the base
// width of the complete tree, not the width of the current branch.
func MerkleBranchHash(depth uint8, width uint32, left, right [32]byte) [32]byte {
	return merkleNode(2, depth, width, left, right)
}

func MerkleRoot(leaves [][32]byte) [32]byte {
	width := uint32(len(leaves))
	if width == 1 {
		return leaves[0]
	}
	return merklize(leaves, 0, width, width)
}

func merklize(leaves [][32]byte, depth uint8, branchWidth, baseWidth uint32) [32]byte {
	if branchWidth <= 2 {
		switch len(leaves) {
		case 0:
			return merkleNode(0, depth, baseWidth, virtualMerkleLeaf, virtualMerkleLeaf)
		case 1:
			return merkleNode(1, depth, baseWidth, leaves[0], virtualMerkleLeaf)
		default:
			return merkleNode(2, depth, baseWidth, leaves[0], leaves[1])
		}
	}
	leftWidth := branchWidth/2 + branchWidth%2
	left := merklize(leaves[:leftWidth], depth+1, leftWidth, baseWidth)
	right := merklize(leaves[leftWidth:], depth+1, branchWidth-leftWidth, baseWidth)
	return merkleNode(2, depth, baseWidth, left, right)
}

func merkleNode(branching, depth uint8, width uint32, node1, node2 [32]byte) [32]byte {
	var encoded [98]byte
	encoded[0] = branching
	encoded[1] = depth
	// amplify::u256 strict encoding is a 32-byte little-endian integer.
	binary.LittleEndian.PutUint32(encoded[2:6], width)
	copy(encoded[34:66], node1[:])
	copy(encoded[66:98], node2[:])
	return TaggedHash(MerkleNodeCommitmentTag, encoded[:])
}
