package consignment

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/sat20-labs/rgb11/strict_types"
)

var (
	ErrWitnessUnresolved = errors.New("RGB11 Bitcoin witness is unresolved")
	ErrWitnessArchived   = errors.New("RGB11 Bitcoin witness is archived by reorg")
	ErrWitnessMismatch   = errors.New("RGB11 Bitcoin witness transaction mismatch")
	ErrOutpointUnknown   = errors.New("RGB11 state outpoint status is unknown")
	ErrOutpointSpend     = errors.New("RGB11 state outpoint has an inconsistent spend")
)

type WitnessState uint8

const (
	WitnessUnknown WitnessState = iota
	WitnessTentative
	WitnessMined
	WitnessArchived
)

type Outpoint struct {
	TxID [32]byte
	Vout uint32
}

type WitnessEvidence struct {
	RawTx       []byte
	State       WitnessState
	BlockHeight uint32
	BlockHash   string
}

type OutpointEvidence struct {
	Known        bool
	Exists       bool
	Spent        bool
	SpendingTxID *[32]byte
}

// BitcoinResolver is intentionally limited to chain facts. Implementations
// may use the Indexer v3 rawtx/status/outspend endpoints, but must never return
// an RGB balance or other client-side projection.
type BitcoinResolver interface {
	ResolveRGB11Witness(txid [32]byte) (WitnessEvidence, error)
	ResolveRGB11Outpoint(outpoint Outpoint) (OutpointEvidence, error)
}

func resolveWitness(value strict_types.Value, resolver BitcoinResolver) (*wire.MsgTx, [32]byte, WitnessEvidence, error) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueUnion || value.Inner == nil {
		return nil, [32]byte{}, WitnessEvidence{}, ErrWitnessUnresolved
	}
	var embedded *wire.MsgTx
	var expected [32]byte
	switch value.Name {
	case "tx":
		tx, err := txFromStrictValue(value.Inner.Unwrap())
		if err != nil {
			return nil, [32]byte{}, WitnessEvidence{}, err
		}
		embedded = tx
		expected = hashArray(tx.TxHash())
	case "txid":
		raw, ok := value.Inner.Bytes()
		if !ok || len(raw) != 32 {
			return nil, [32]byte{}, WitnessEvidence{}, ErrWitnessUnresolved
		}
		copy(expected[:], raw)
	default:
		return nil, [32]byte{}, WitnessEvidence{}, ErrWitnessUnresolved
	}
	if resolver == nil {
		return nil, expected, WitnessEvidence{}, ErrWitnessUnresolved
	}
	evidence, err := resolver.ResolveRGB11Witness(expected)
	if err != nil {
		return nil, expected, evidence, fmt.Errorf("%w: %v", ErrWitnessUnresolved, err)
	}
	if evidence.State == WitnessArchived {
		return nil, expected, evidence, ErrWitnessArchived
	}
	if evidence.State != WitnessTentative && evidence.State != WitnessMined {
		return nil, expected, evidence, ErrWitnessUnresolved
	}
	resolved := new(wire.MsgTx)
	if len(evidence.RawTx) == 0 || resolved.Deserialize(bytes.NewReader(evidence.RawTx)) != nil {
		return nil, expected, evidence, ErrWitnessUnresolved
	}
	if actual := hashArray(resolved.TxHash()); actual != expected {
		return nil, expected, evidence, ErrWitnessMismatch
	}
	if embedded != nil && hashArray(embedded.TxHash()) != hashArray(resolved.TxHash()) {
		return nil, expected, evidence, ErrWitnessMismatch
	}
	return resolved, expected, evidence, nil
}

func txFromStrictValue(value strict_types.Value) (*wire.MsgTx, error) {
	versionValue, versionOK := value.Field("version")
	inputsValue, inputsOK := value.Field("inputs")
	outputsValue, outputsOK := value.Field("outputs")
	lockTimeValue, lockTimeOK := value.Field("lockTime")
	version, versionNumberOK := signedNumber(versionValue)
	lockTime, lockTimeNumberOK := unsignedNumber(lockTimeValue)
	inputs := inputsValue.Unwrap()
	outputs := outputsValue.Unwrap()
	if !versionOK || !inputsOK || !outputsOK || !lockTimeOK || !versionNumberOK || !lockTimeNumberOK ||
		version < math.MinInt32 || version > math.MaxInt32 || lockTime > math.MaxUint32 ||
		inputs.Kind != strict_types.ValueList || outputs.Kind != strict_types.ValueList {
		return nil, ErrWitnessUnresolved
	}
	tx := wire.NewMsgTx(int32(version))
	tx.LockTime = uint32(lockTime)
	for _, inputValue := range inputs.Items {
		inputValue = inputValue.Unwrap()
		prevOutput, prevOK := inputValue.Field("prevOutput")
		sigScript, scriptOK := inputValue.Field("sigScript")
		sequenceValue, sequenceOK := inputValue.Field("sequence")
		witnessValue, witnessOK := inputValue.Field("witness")
		txidValue, txidOK := prevOutput.Unwrap().Field("txid")
		voutValue, voutOK := prevOutput.Unwrap().Field("vout")
		txid, txidBytesOK := txidValue.Bytes()
		vout, voutNumberOK := unsignedNumber(voutValue)
		sequence, sequenceNumberOK := unsignedNumber(sequenceValue)
		script, sigBytesOK := sigScript.Bytes()
		witnessValue = witnessValue.Unwrap()
		if !prevOK || !scriptOK || !sequenceOK || !witnessOK || !txidOK || !voutOK || !txidBytesOK || len(txid) != 32 ||
			!voutNumberOK || vout > math.MaxUint32 || !sequenceNumberOK || sequence > math.MaxUint32 || !sigBytesOK ||
			witnessValue.Kind != strict_types.ValueList {
			return nil, ErrWitnessUnresolved
		}
		var hash chainhash.Hash
		copy(hash[:], txid)
		txIn := wire.NewTxIn(&wire.OutPoint{Hash: hash, Index: uint32(vout)}, script, nil)
		txIn.Sequence = uint32(sequence)
		for _, witnessItem := range witnessValue.Items {
			raw, ok := witnessItem.Bytes()
			if !ok {
				return nil, ErrWitnessUnresolved
			}
			txIn.Witness = append(txIn.Witness, raw)
		}
		tx.AddTxIn(txIn)
	}
	for _, outputValue := range outputs.Items {
		outputValue = outputValue.Unwrap()
		amountValue, amountOK := outputValue.Field("value")
		scriptValue, scriptOK := outputValue.Field("scriptPubkey")
		amount, amountNumberOK := unsignedNumber(amountValue)
		script, scriptBytesOK := scriptValue.Bytes()
		if !amountOK || !scriptOK || !amountNumberOK || amount > math.MaxInt64 || !scriptBytesOK {
			return nil, ErrWitnessUnresolved
		}
		tx.AddTxOut(wire.NewTxOut(int64(amount), script))
	}
	return tx, nil
}

func hashArray(hash chainhash.Hash) [32]byte {
	return [32]byte(hash)
}

func unsignedNumber(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return unsignedNumber(*value.Inner)
	}
	return value.Uint64()
}

func signedNumber(value strict_types.Value) (int64, bool) {
	value = value.Unwrap()
	if value.Signed != nil {
		return *value.Signed, true
	}
	if value.Unsigned != nil && *value.Unsigned <= math.MaxInt64 {
		return int64(*value.Unsigned), true
	}
	return 0, false
}
