package anchors

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math/big"

	"github.com/sat20-labs/rgb11/consensus"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/commit_verify/mpc/block.rs
// Upstream-File: src/commit_verify/mpc/tree.rs
// Translation-Revision: 1

const MPCCommitmentTag = "urn:ubideco:mpc:commitment#2024-01-31"

var ErrInvalidMPCProof = errors.New("invalid RGB11 LNPBP-4 proof")

type MPCProof struct {
	Position uint32
	Cofactor uint16
	Path     [][32]byte
}

// NewMPCProof creates a privacy-preserving proof skeleton for a single RGB
// protocol message. Partner subtree hashes are random commitments; they do not
// disclose or imply additional protocol messages.
func NewMPCProof(protocolID [32]byte, depth uint8) (MPCProof, error) {
	return NewMPCProofFrom(protocolID, depth, rand.Reader)
}

func NewMPCProofFrom(protocolID [32]byte, depth uint8, entropy io.Reader) (MPCProof, error) {
	if depth > 31 || entropy == nil {
		return MPCProof{}, ErrInvalidMPCProof
	}
	width := uint32(1) << depth
	proof := MPCProof{Position: protocolPosition(protocolID, width), Path: make([][32]byte, depth)}
	for index := range proof.Path {
		if _, err := io.ReadFull(entropy, proof.Path[index][:]); err != nil {
			return MPCProof{}, err
		}
	}
	return proof, nil
}

// ConvolveMPC proves that message is committed under protocolID and returns
// the commitment that must be embedded by the Bitcoin DBC proof.
func ConvolveMPC(proof MPCProof, protocolID, message [32]byte) ([32]byte, error) {
	depth := len(proof.Path)
	if depth > 31 {
		return [32]byte{}, ErrInvalidMPCProof
	}
	width := uint32(1) << uint(depth)
	factoredWidth := width - uint32(proof.Cofactor)
	if factoredWidth == 0 || proof.Position >= width || protocolPosition(protocolID, factoredWidth) != proof.Position {
		return [32]byte{}, ErrInvalidMPCProof
	}

	// Leaf::Inhabited has custom strict union tag 0x10 followed by the raw
	// ProtocolId and Message newtypes.
	leaf := make([]byte, 65)
	leaf[0] = 0x10
	copy(leaf[1:33], protocolID[:])
	copy(leaf[33:], message[:])
	current := consensus.MerkleLeafHash(leaf)
	for index := depth - 1; index >= 0; index-- {
		partner := proof.Path[index]
		shift := uint(depth - 1 - index)
		if (proof.Position>>shift)&1 == 0 {
			current = consensus.MerkleBranchHash(uint8(index), width, current, partner)
		} else {
			current = consensus.MerkleBranchHash(uint8(index), width, partner, current)
		}
	}
	encoded := make([]byte, 35)
	encoded[0] = byte(depth) // amplify::u5 strict encoding
	binary.LittleEndian.PutUint16(encoded[1:3], proof.Cofactor)
	copy(encoded[3:], current[:])
	return consensus.TaggedHash(MPCCommitmentTag, encoded), nil
}

func protocolPosition(protocolID [32]byte, width uint32) uint32 {
	bigEndian := make([]byte, len(protocolID))
	for index := range protocolID {
		bigEndian[len(protocolID)-1-index] = protocolID[index]
	}
	value := new(big.Int).SetBytes(bigEndian)
	value.Mod(value, new(big.Int).SetUint64(uint64(width)))
	return uint32(value.Uint64())
}
