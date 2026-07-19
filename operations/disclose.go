package operations

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

const DiscloseCommitmentTag = "urn:lnp-bp:rgb:disclose#2024-02-16"

var ErrInvalidDisclosure = errors.New("invalid RGB11 operation disclosure")

type discloseEntry struct {
	assignmentType uint16
	index          uint16
	value          []byte
}

// DiscloseHash commits to exactly the operation details revealed in a
// consignment. It is distinct from the operation id, which commits to the
// concealed state graph.
func DiscloseHash(operation strict_types.Value) ([32]byte, error) {
	encoded, err := EncodeDisclose(operation)
	if err != nil {
		return [32]byte{}, err
	}
	return consensus.TaggedHash(DiscloseCommitmentTag, encoded), nil
}

// EncodeDisclose returns the canonical strict encoding committed by
// DiscloseHash. It is exposed for cross-implementation differential tests.
func EncodeDisclose(operation strict_types.Value) ([]byte, error) {
	var operationID [32]byte
	_, isGenesis := operation.Field("schemaId")
	if isGenesis {
		commitment, err := CommitGenesis(operation)
		if err != nil {
			return nil, err
		}
		operationID = commitment.OperationID
	} else {
		commitment, err := CommitTransition(operation)
		if err != nil {
			return nil, err
		}
		operationID = commitment.OperationID
	}
	assignments, ok := operation.Field("assignments")
	assignments = assignments.Unwrap()
	if !ok || assignments.Kind != strict_types.ValueMap {
		return nil, ErrInvalidDisclosure
	}
	var seals, fungible, data []discloseEntry
	for _, typedEntry := range assignments.Entries {
		typeNumber, ok := typedEntry.Key.Unwrap().Uint64()
		typed := typedEntry.Value.Unwrap()
		if !ok || typeNumber > 0xffff || typed.Kind != strict_types.ValueUnion || typed.Inner == nil {
			return nil, ErrInvalidDisclosure
		}
		items := typed.Inner.Unwrap()
		if items.Kind != strict_types.ValueList || len(items.Items) > 0xffff {
			return nil, ErrInvalidDisclosure
		}
		for index, assignment := range items.Items {
			assignment = assignment.Unwrap()
			if assignment.Kind != strict_types.ValueUnion || assignment.Inner == nil {
				return nil, ErrInvalidDisclosure
			}
			fields := assignment.Inner.Unwrap()
			state, stateOK := fields.Field("state")
			seal, sealOK := fields.Field("seal")
			if !stateOK || !sealOK {
				return nil, ErrInvalidDisclosure
			}
			entry := discloseEntry{assignmentType: uint16(typeNumber), index: uint16(index), value: append([]byte(nil), state.Encoded...)}
			if assignment.Name == "revealed" {
				secret, err := disclosureSecret(seal, isGenesis)
				if err != nil {
					return nil, err
				}
				seals = append(seals, discloseEntry{assignmentType: entry.assignmentType, index: entry.index, value: secret[:]})
			}
			switch typed.Name {
			case "declarative":
			case "fungible":
				fungible = append(fungible, entry)
			case "structured":
				data = append(data, entry)
			default:
				return nil, fmt.Errorf("%w: class %s", ErrInvalidDisclosure, typed.Name)
			}
		}
	}
	encoded := append([]byte(nil), operationID[:]...)
	encoded = appendDisclosureMap(encoded, seals)
	encoded = appendDisclosureMap(encoded, fungible)
	encoded = appendDisclosureMap(encoded, data)
	return encoded, nil
}

// Operation::disclose in the frozen Rust consensus code first normalizes
// genesis SingleBlindSeal<Txid> values into graph ChainBlindSeal<TxPtr>
// values. Consequently, revealed genesis seals commit to the TxPtr::Txid tag
// in disclosures even though the genesis operation id commits to the original
// untagged Txid seal.
func disclosureSecret(value strict_types.Value, isGenesis bool) ([32]byte, error) {
	if !isGenesis {
		return assignmentSecret("revealed", value)
	}
	value = unwrap(value)
	txidValue, err := requiredField(value, "txid")
	if err != nil {
		return [32]byte{}, err
	}
	txid, ok := asBytes(txidValue)
	if !ok || len(txid) != 32 {
		return [32]byte{}, ErrInvalidDisclosure
	}
	voutValue, err := requiredField(value, "vout")
	if err != nil {
		return [32]byte{}, err
	}
	vout, ok := asUint(voutValue)
	if !ok || vout > uint64(^uint32(0)) {
		return [32]byte{}, ErrInvalidDisclosure
	}
	blindingValue, err := requiredField(value, "blinding")
	if err != nil {
		return [32]byte{}, err
	}
	blinding, ok := asUint(blindingValue)
	if !ok {
		return [32]byte{}, ErrInvalidDisclosure
	}
	graph, err := seals.NewGraphBlindSeal(txid, uint32(vout), blinding)
	if err != nil {
		return [32]byte{}, err
	}
	secret, err := graph.Conceal()
	return [32]byte(secret), err
}

func appendDisclosureMap(encoded []byte, entries []discloseEntry) []byte {
	length := len(entries)
	encoded = append(encoded, byte(length), byte(length>>8), byte(length>>16))
	for _, entry := range entries {
		var key [4]byte
		binary.LittleEndian.PutUint16(key[:2], entry.assignmentType)
		binary.LittleEndian.PutUint16(key[2:], entry.index)
		encoded = append(encoded, key[:]...)
		encoded = append(encoded, entry.value...)
	}
	return encoded
}
