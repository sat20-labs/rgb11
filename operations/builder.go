package operations

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/btcsuite/btcd/wire"
	"github.com/sat20-labs/rgb11/anchors"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

var ErrBuildTransition = errors.New("unable to build RGB11 transition")

type TransitionInput struct {
	OperationID    [32]byte
	AssignmentType uint16
	Index          uint16
}

type TransitionOutput struct {
	AssignmentType uint16
	Class          string
	Amount         uint64
	Data           []byte
	SecretSeal     [32]byte
	RevealedSeal   *seals.GraphBlindSeal
}

type TransitionSpec struct {
	ContractID     [32]byte
	Nonce          uint64
	TransitionType uint16
	Metadata       map[uint16][]byte
	Globals        map[uint16][][]byte
	Inputs         []TransitionInput
	Outputs        []TransitionOutput
	Signature      []byte
}

// BuildTransition creates the canonical strict Transition representation used
// by the standard RGB11 transfer schemas. Outputs are sorted by concealed seal,
// exactly as Assign implements Ord in the frozen Rust consensus library.
func BuildTransition(spec TransitionSpec) (strict_types.Value, TransitionCommitment, error) {
	if len(spec.Inputs) == 0 || len(spec.Outputs) == 0 || len(spec.Inputs) > 0xffff || len(spec.Outputs) > 0xffff ||
		(len(spec.Signature) != 0 && len(spec.Signature) != 64) {
		return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
	}
	inputs := append([]TransitionInput(nil), spec.Inputs...)
	sort.Slice(inputs, func(i, j int) bool { return compareOpout(inputs[i], inputs[j]) < 0 })
	for index := 1; index < len(inputs); index++ {
		if compareOpout(inputs[index-1], inputs[index]) == 0 {
			return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
		}
	}

	groups := make(map[uint16][]encodedTransitionOutput)
	classes := make(map[uint16]string)
	for _, output := range spec.Outputs {
		if output.Class != "declarative" && output.Class != "fungible" && output.Class != "structured" {
			return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
		}
		if previous, ok := classes[output.AssignmentType]; ok && previous != output.Class {
			return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
		}
		if output.Class == "structured" && len(output.Data) > 0xffff {
			return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
		}
		assignment, err := encodeTransitionOutput(output)
		if err != nil {
			return strict_types.Value{}, TransitionCommitment{}, err
		}
		classes[output.AssignmentType] = output.Class
		groups[output.AssignmentType] = append(groups[output.AssignmentType], assignment)
	}
	types := make([]int, 0, len(groups))
	for assignmentType := range groups {
		types = append(types, int(assignmentType))
	}
	sort.Ints(types)

	var encoded bytes.Buffer
	encoded.Write([]byte{0, 0}) // Ffv::v0 tuple(enum)
	encoded.Write(spec.ContractID[:])
	writeU64(&encoded, spec.Nonce)
	writeU16(&encoded, spec.TransitionType)
	if err := writeMetadata(&encoded, spec.Metadata); err != nil {
		return strict_types.Value{}, TransitionCommitment{}, err
	}
	if err := writeGlobals(&encoded, spec.Globals); err != nil {
		return strict_types.Value{}, TransitionCommitment{}, err
	}
	writeU16(&encoded, uint16(len(inputs)))
	for _, input := range inputs {
		encoded.Write(input.OperationID[:])
		writeU16(&encoded, input.AssignmentType)
		writeU16(&encoded, input.Index)
	}
	writeU16(&encoded, uint16(len(types)))
	for _, typeNumber := range types {
		assignmentType := uint16(typeNumber)
		outputs := groups[assignmentType]
		sort.Slice(outputs, func(i, j int) bool {
			return bytes.Compare(outputs[i].encoded, outputs[j].encoded) < 0
		})
		for index := 1; index < len(outputs); index++ {
			if bytes.Equal(outputs[index-1].encoded, outputs[index].encoded) {
				return strict_types.Value{}, TransitionCommitment{}, ErrBuildTransition
			}
		}
		writeU16(&encoded, assignmentType)
		switch classes[assignmentType] {
		case "declarative":
			encoded.WriteByte(0)
		case "fungible":
			encoded.WriteByte(1)
		case "structured":
			encoded.WriteByte(2)
		}
		writeU16(&encoded, uint16(len(outputs)))
		for _, output := range outputs {
			encoded.Write(output.encoded)
		}
	}
	if len(spec.Signature) == 0 {
		encoded.WriteByte(0)
	} else {
		encoded.WriteByte(1)
		encoded.Write(spec.Signature)
	}

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return strict_types.Value{}, TransitionCommitment{}, err
	}
	value, err := registry.Decode("RGBCommit", "Transition", encoded.Bytes())
	if err != nil {
		return strict_types.Value{}, TransitionCommitment{}, err
	}
	commitment, err := CommitTransition(value)
	if err != nil {
		return strict_types.Value{}, TransitionCommitment{}, err
	}
	return value, commitment, nil
}

type encodedTransitionOutput struct {
	encoded []byte
}

func encodeTransitionOutput(output TransitionOutput) (encodedTransitionOutput, error) {
	secretSet := output.SecretSeal != [32]byte{}
	if secretSet == (output.RevealedSeal != nil) {
		return encodedTransitionOutput{}, ErrBuildTransition
	}
	var encoded bytes.Buffer
	if output.RevealedSeal != nil {
		seal, err := output.RevealedSeal.StrictBytes()
		if err != nil {
			return encodedTransitionOutput{}, ErrBuildTransition
		}
		encoded.WriteByte(0) // Assign::Revealed
		encoded.Write(seal)
	} else {
		encoded.WriteByte(1) // Assign::ConfidentialSeal
		encoded.Write(output.SecretSeal[:])
	}
	switch output.Class {
	case "declarative":
	case "fungible":
		encoded.WriteByte(8) // FungibleState::Bits64
		writeU64(&encoded, output.Amount)
	case "structured":
		writeU16(&encoded, uint16(len(output.Data)))
		encoded.Write(output.Data)
	default:
		return encodedTransitionOutput{}, ErrBuildTransition
	}
	return encodedTransitionOutput{encoded: encoded.Bytes()}, nil
}

func writeMetadata(encoded *bytes.Buffer, metadata map[uint16][]byte) error {
	if len(metadata) > 0xff {
		return ErrBuildTransition
	}
	types := sortedStateTypes(metadata)
	encoded.WriteByte(byte(len(types)))
	for _, typeID := range types {
		value := metadata[typeID]
		if len(value) > 0xffff {
			return ErrBuildTransition
		}
		writeU16(encoded, typeID)
		writeU16(encoded, uint16(len(value)))
		encoded.Write(value)
	}
	return nil
}

func writeGlobals(encoded *bytes.Buffer, globals map[uint16][][]byte) error {
	if len(globals) > 0xff {
		return ErrBuildTransition
	}
	types := make([]int, 0, len(globals))
	for typeID := range globals {
		types = append(types, int(typeID))
	}
	sort.Ints(types)
	encoded.WriteByte(byte(len(types)))
	for _, number := range types {
		typeID := uint16(number)
		values := globals[typeID]
		if len(values) == 0 || len(values) > 0xffff {
			return ErrBuildTransition
		}
		writeU16(encoded, typeID)
		writeU16(encoded, uint16(len(values)))
		for _, value := range values {
			if len(value) > 0xffff {
				return ErrBuildTransition
			}
			writeU16(encoded, uint16(len(value)))
			encoded.Write(value)
		}
	}
	return nil
}

func sortedStateTypes(values map[uint16][]byte) []uint16 {
	types := make([]int, 0, len(values))
	for typeID := range values {
		types = append(types, int(typeID))
	}
	sort.Ints(types)
	result := make([]uint16, len(types))
	for index, typeID := range types {
		result[index] = uint16(typeID)
	}
	return result
}

// BuildBundle commits one transition to every state input it consumes.
func BuildBundle(inputs []TransitionInput, transition strict_types.Value) (strict_types.Value, BundleCommitment, error) {
	commitment, err := CommitTransition(transition)
	if err != nil || len(inputs) == 0 || len(inputs) > 0xffff {
		return strict_types.Value{}, BundleCommitment{}, ErrBuildTransition
	}
	inputs = append([]TransitionInput(nil), inputs...)
	sort.Slice(inputs, func(i, j int) bool { return compareOpout(inputs[i], inputs[j]) < 0 })
	var encoded bytes.Buffer
	writeU16(&encoded, uint16(len(inputs)))
	for _, input := range inputs {
		encoded.Write(input.OperationID[:])
		writeU16(&encoded, input.AssignmentType)
		writeU16(&encoded, input.Index)
		encoded.Write(commitment.OperationID[:])
	}
	writeU16(&encoded, 1)
	encoded.Write(commitment.OperationID[:])
	encoded.Write(transition.Encoded)

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return strict_types.Value{}, BundleCommitment{}, err
	}
	value, err := registry.Decode("RGBCommit", "TransitionBundle", encoded.Bytes())
	if err != nil {
		return strict_types.Value{}, BundleCommitment{}, err
	}
	bundle, err := CommitBundle(value)
	return value, bundle, err
}

// BuildOpretWitnessBundle links a transition bundle to its Bitcoin witness.
func BuildOpretWitnessBundle(witnessTxID [32]byte, bundle strict_types.Value, proof anchors.MPCProof) (strict_types.Value, error) {
	if len(proof.Path) > 31 {
		return strict_types.Value{}, ErrBuildTransition
	}
	var encoded bytes.Buffer
	encoded.WriteByte(0) // PubWitness::Txid
	encoded.Write(witnessTxID[:])
	writeU32(&encoded, proof.Position)
	writeU16(&encoded, proof.Cofactor)
	encoded.WriteByte(byte(len(proof.Path)))
	for _, node := range proof.Path {
		encoded.Write(node[:])
	}
	encoded.WriteByte(2) // DbcProof::Opret(OpretProof)
	encoded.Write(bundle.Encoded)
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return strict_types.Value{}, err
	}
	return registry.Decode("RGBStd", "WitnessBundle", encoded.Bytes())
}

// BuildOpretWitnessBundleWithTx embeds the complete public witness transaction.
// Witness recipients need PubWitness::Tx (not only PubWitness::Txid) so an
// independent RGB wallet can match the invoice script to the assigned vout
// before the transaction has been broadcast.
func BuildOpretWitnessBundleWithTx(tx *wire.MsgTx, bundle strict_types.Value, proof anchors.MPCProof) (strict_types.Value, error) {
	if tx == nil || len(proof.Path) > 31 {
		return strict_types.Value{}, ErrBuildTransition
	}
	var encoded bytes.Buffer
	encoded.WriteByte(1) // PubWitness::Tx
	writeStrictBitcoinTx(&encoded, tx)
	writeU32(&encoded, proof.Position)
	writeU16(&encoded, proof.Cofactor)
	encoded.WriteByte(byte(len(proof.Path)))
	for _, node := range proof.Path {
		encoded.Write(node[:])
	}
	encoded.WriteByte(2) // DbcProof::Opret(OpretProof)
	encoded.Write(bundle.Encoded)
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return strict_types.Value{}, err
	}
	return registry.Decode("RGBStd", "WitnessBundle", encoded.Bytes())
}

// writeStrictBitcoinTx mirrors rgb-strict-encoding 1.0.2's Bitcoin Tx,
// TxIn, TxOut, ScriptBuf and Witness encoders. Strict collections use fixed
// little-endian u32 lengths, unlike Bitcoin consensus CompactSize encoding.
func writeStrictBitcoinTx(encoded *bytes.Buffer, tx *wire.MsgTx) {
	_ = binary.Write(encoded, binary.LittleEndian, tx.Version)
	writeU32(encoded, uint32(len(tx.TxIn)))
	for _, input := range tx.TxIn {
		encoded.Write(input.PreviousOutPoint.Hash[:])
		writeU32(encoded, input.PreviousOutPoint.Index)
		writeU32(encoded, uint32(len(input.SignatureScript)))
		encoded.Write(input.SignatureScript)
		writeU32(encoded, input.Sequence)
		writeU32(encoded, uint32(len(input.Witness)))
		for _, item := range input.Witness {
			writeU32(encoded, uint32(len(item)))
			encoded.Write(item)
		}
	}
	writeU32(encoded, uint32(len(tx.TxOut)))
	for _, output := range tx.TxOut {
		writeU64(encoded, uint64(output.Value))
		writeU32(encoded, uint32(len(output.PkScript)))
		encoded.Write(output.PkScript)
	}
	writeU32(encoded, tx.LockTime)
}

func compareOpout(left, right TransitionInput) int {
	if order := bytes.Compare(left.OperationID[:], right.OperationID[:]); order != 0 {
		return order
	}
	if left.AssignmentType < right.AssignmentType {
		return -1
	}
	if left.AssignmentType > right.AssignmentType {
		return 1
	}
	if left.Index < right.Index {
		return -1
	}
	if left.Index > right.Index {
		return 1
	}
	return 0
}

func writeU16(buf *bytes.Buffer, value uint16) { _ = binary.Write(buf, binary.LittleEndian, value) }
func writeU32(buf *bytes.Buffer, value uint32) { _ = binary.Write(buf, binary.LittleEndian, value) }
func writeU64(buf *bytes.Buffer, value uint64) { _ = binary.Write(buf, binary.LittleEndian, value) }
