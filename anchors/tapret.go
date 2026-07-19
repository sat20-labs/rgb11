package anchors

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/dbc/tapret/tapscript.rs
// Translation-Revision: 1

var (
	ErrInvalidTapret = errors.New("invalid RGB11 tapret commitment")
	tapretPrefix     = [31]byte{
		0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50,
		0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50,
		0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x50, 0x6a,
		0x21,
	}
)

type TapretPartnerKind uint8

const (
	TapretLeftNode TapretPartnerKind = iota
	TapretRightLeaf
	TapretRightBranch
)

// TapretPartner carries the single deterministic sibling allowed by the
// TapretFirst proof. Fields used depend on Kind.
type TapretPartner struct {
	Kind          TapretPartnerKind
	NodeHash      [32]byte
	LeafVersion   byte
	LeafScript    []byte
	LeftNodeHash  [32]byte
	RightNodeHash [32]byte
}

// TapretScript returns the 64-byte tapscript leaf containing the MPC
// commitment and nonce.
func TapretScript(commitment [32]byte, nonce uint8) []byte {
	script := make([]byte, 64)
	copy(script, tapretPrefix[:])
	copy(script[31:63], commitment[:])
	script[63] = nonce
	return script
}

// TapretOutputKey implements the root-path Tapret proof used by a new SAT20
// carrier output. More complex proofs with a partner node are validated by the
// full consensus validator before projection.
func TapretOutputKey(internalXOnly [32]byte, commitment [32]byte, nonce uint8) ([32]byte, error) {
	return TapretOutputKeyWithPartner(internalXOnly, commitment, nonce, nil)
}

// TapretOutputKeyWithPartner validates a frozen 0.11 Tapret path proof and
// reconstructs the tweaked x-only output key.
func TapretOutputKeyWithPartner(internalXOnly [32]byte, commitment [32]byte, nonce uint8, partner *TapretPartner) ([32]byte, error) {
	compressed := append([]byte{0x02}, internalXOnly[:]...)
	internal, err := btcec.ParsePubKey(compressed)
	if err != nil {
		return [32]byte{}, ErrInvalidTapret
	}
	merkleRoot, err := TapretMerkleRoot(commitment, nonce, partner)
	if err != nil {
		return [32]byte{}, err
	}
	tweakInput := append(append([]byte{}, internalXOnly[:]...), merkleRoot[:]...)
	tweakHash := taggedHash("TapTweak", tweakInput)
	var tweak btcec.ModNScalar
	if overflow := tweak.SetByteSlice(tweakHash[:]); overflow {
		return [32]byte{}, ErrInvalidTapret
	}
	var internalPoint, tweakPoint, outputPoint btcec.JacobianPoint
	internal.AsJacobian(&internalPoint)
	btcec.ScalarBaseMultNonConst(&tweak, &tweakPoint)
	btcec.AddNonConst(&internalPoint, &tweakPoint, &outputPoint)
	if outputPoint.Z.IsZero() {
		return [32]byte{}, ErrInvalidTapret
	}
	outputPoint.ToAffine()
	output := btcec.NewPublicKey(&outputPoint.X, &outputPoint.Y)
	serialized := output.X().Bytes()
	var result [32]byte
	copy(result[32-len(serialized):], serialized)
	return result, nil
}

// TapretMerkleRoot reconstructs the root committed by a TapretFirst proof.
// Wallet adapters persist it with the carrier binding so the tweaked key can
// be recovered and signed without reinterpreting the consignment.
func TapretMerkleRoot(commitment [32]byte, nonce uint8, partner *TapretPartner) ([32]byte, error) {
	leafHash := tapLeafHash(0xc0, TapretScript(commitment, nonce))
	if partner == nil {
		return leafHash, nil
	}
	partnerHash, err := partner.validatedHash(leafHash)
	if err != nil {
		return [32]byte{}, err
	}
	return tapBranchHash(leafHash, partnerHash), nil
}

func (p TapretPartner) validatedHash(commitmentLeaf [32]byte) ([32]byte, error) {
	var hash [32]byte
	switch p.Kind {
	case TapretLeftNode:
		hash = p.NodeHash
		if bytes.Compare(hash[:], commitmentLeaf[:]) > 0 {
			return [32]byte{}, ErrInvalidTapret
		}
	case TapretRightLeaf:
		if len(p.LeafScript) >= len(tapretPrefix) && bytes.Equal(p.LeafScript[:len(tapretPrefix)], tapretPrefix[:]) {
			return [32]byte{}, ErrInvalidTapret
		}
		hash = tapLeafHash(p.LeafVersion, p.LeafScript)
		if bytes.Compare(commitmentLeaf[:], hash[:]) > 0 {
			return [32]byte{}, ErrInvalidTapret
		}
	case TapretRightBranch:
		if bytes.Compare(p.LeftNodeHash[:], p.RightNodeHash[:]) > 0 ||
			bytes.Equal(p.LeftNodeHash[:31], tapretPrefix[:31]) {
			return [32]byte{}, ErrInvalidTapret
		}
		hash = tapBranchHash(p.LeftNodeHash, p.RightNodeHash)
		if bytes.Compare(commitmentLeaf[:], hash[:]) > 0 {
			return [32]byte{}, ErrInvalidTapret
		}
	default:
		return [32]byte{}, ErrInvalidTapret
	}
	return hash, nil
}

func tapLeafHash(version byte, script []byte) [32]byte {
	input := make([]byte, 0, len(script)+10)
	input = append(input, version)
	input = appendCompactSize(input, uint64(len(script)))
	input = append(input, script...)
	return taggedHash("TapLeaf", input)
}

func appendCompactSize(dst []byte, value uint64) []byte {
	switch {
	case value < 0xfd:
		return append(dst, byte(value))
	case value <= 0xffff:
		dst = append(dst, 0xfd, 0, 0)
		binary.LittleEndian.PutUint16(dst[len(dst)-2:], uint16(value))
	case value <= 0xffffffff:
		dst = append(dst, 0xfe, 0, 0, 0, 0)
		binary.LittleEndian.PutUint32(dst[len(dst)-4:], uint32(value))
	default:
		dst = append(dst, 0xff, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.LittleEndian.PutUint64(dst[len(dst)-8:], value)
	}
	return dst
}

func tapBranchHash(first, second [32]byte) [32]byte {
	if bytes.Compare(first[:], second[:]) > 0 {
		first, second = second, first
	}
	input := append(append(make([]byte, 0, 64), first[:]...), second[:]...)
	return taggedHash("TapBranch", input)
}

func TapretPkScript(outputXOnly [32]byte) []byte {
	script := make([]byte, 34)
	script[0], script[1] = 0x51, 0x20
	copy(script[2:], outputXOnly[:])
	return script
}

func taggedHash(tag string, message []byte) [32]byte {
	tagHash := sha256.Sum256([]byte(tag))
	h := sha256.New()
	h.Write(tagHash[:])
	h.Write(tagHash[:])
	h.Write(message)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
