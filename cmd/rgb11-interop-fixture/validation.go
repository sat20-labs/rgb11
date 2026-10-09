package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/sat20-labs/rgb11/anchors"
	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
)

type witnessFact struct {
	TxID      string `json:"txid"`
	Raw       string `json:"raw_tx"`
	State     string `json:"state"`
	Height    uint32 `json:"height"`
	Timestamp int64  `json:"timestamp"`
}
type validationEvidence struct {
	Chain     string        `json:"chain_net"`
	Witnesses []witnessFact `json:"witnesses"`
	prev      consignment.Outpoint
}

func (e validationEvidence) ResolveRGB11Witness(id [32]byte) (consignment.WitnessEvidence, error) {
	for _, item := range e.Witnesses {
		if chainhash.Hash(id).String() != item.TxID {
			continue
		}
		raw, err := hex.DecodeString(item.Raw)
		if err != nil {
			return consignment.WitnessEvidence{}, err
		}
		state := consignment.WitnessTentative
		switch item.State {
		case "unknown":
			state = consignment.WitnessUnknown
		case "mined":
			state = consignment.WitnessMined
		case "archived":
			state = consignment.WitnessArchived
		}
		return consignment.WitnessEvidence{RawTx: raw, State: state, BlockHeight: item.Height}, nil
	}
	return consignment.WitnessEvidence{}, fmt.Errorf("missing witness evidence")
}
func (e validationEvidence) ResolveRGB11Outpoint(p consignment.Outpoint) (consignment.OutpointEvidence, error) {
	return consignment.OutpointEvidence{Known: true, Exists: true, Spent: p == e.prev}, nil
}

type validationCase struct {
	Binary     bool   `json:"binary,omitempty"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Evidence   string `json:"evidence"`
	Expected   string `json:"expected"`
	GoOutcome  string `json:"go_outcome"`
	GoError    string `json:"go_error,omitempty"`
	GoStage    string `json:"go_stage,omitempty"`
	PayloadHex string `json:"payload_hex,omitempty"`
}

func writeValidationCases(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var cases []validationCase
	add := func(name, armor string, e validationEvidence, expected string) error {
		item := validationCase{Name: name, Path: filepath.Join(dir, name+".rgba"), Evidence: filepath.Join(dir, name+".json"), Expected: expected}
		c, err := consignment.DecodeArmor(armor)
		if err == nil {
			item.PayloadHex = hex.EncodeToString(c.Armor.Data)
			err = c.ValidateHistory(e)
		}
		item.GoOutcome = "valid"
		if err != nil {
			item.GoOutcome = "invalid"
			item.GoError = err.Error()
			if errors.Is(err, schemas.ErrSchemaScript) {
				item.GoStage = "schema_script"
			}
			if errors.Is(err, consignment.ErrWitnessUnresolved) || errors.Is(err, consignment.ErrWitnessMismatch) {
				item.GoOutcome = "evidence_error"
			}
		}
		if item.GoOutcome != expected {
			return fmt.Errorf("%s: Go outcome=%s want=%s: %v", name, item.GoOutcome, expected, err)
		}
		if err := os.WriteFile(item.Path, []byte(armor), 0o600); err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(e, "", "  ")
		if err := os.WriteFile(item.Evidence, raw, 0o600); err != nil {
			return err
		}
		cases = append(cases, item)
		return nil
	}
	for _, kind := range []string{"nia", "ifa", "cfa", "uda"} {
		for _, mutation := range []string{"valid", "invalid-state", "missing-history", "seal-mismatch", "anchor-mismatch"} {
			armor, e, err := buildValidationTransfer(kind, mutation)
			if err != nil {
				return err
			}
			expected := "invalid"
			if mutation == "valid" {
				expected = "valid"
			}
			if err := add(kind+"-"+mutation, armor, e, expected); err != nil {
				return err
			}
			if mutation == "valid" {
				e.Witnesses[0].State = "mined"
				if err := add(kind+"-mined", armor, e, "valid"); err != nil {
					return err
				}
				e.Witnesses[0].State = "archived"
				if err := add(kind+"-archived", armor, e, "invalid"); err != nil {
					return err
				}
				e.Witnesses[0].State = "unknown"
				if err := add(kind+"-unresolved", armor, e, "evidence_error"); err != nil {
					return err
				}
				e.Witnesses[0].State = "tentative"
				raw, _ := hex.DecodeString(e.Witnesses[0].Raw)
				raw[len(raw)-1] ^= 1 // change locktime, keeping the requested txid fixed
				e.Witnesses[0].Raw = hex.EncodeToString(raw)
				if err := add(kind+"-witness-mismatch", armor, e, "evidence_error"); err != nil {
					return err
				}
			}
		}
	}
	// IFA inflation uses a real right, then exceeds the remaining authority.
	for _, mutation := range []string{"inflate-valid", "inflate-excess", "inflate-no-right"} {
		armor, e, err := buildValidationTransfer("ifa", mutation)
		if err != nil {
			return err
		}
		expected := "invalid"
		if mutation == "inflate-valid" {
			expected = "valid"
		}
		if err := add("ifa-"+mutation, armor, e, expected); err != nil {
			return err
		}
	}
	armor, evidence, err := buildValidationTransfer("nia", "valid")
	if err != nil {
		return err
	}
	base, err := consignment.DecodeArmor(armor)
	if err != nil {
		return err
	}
	for _, name := range []string{"truncated", "trailing-byte", "invalid-enum", "noncanonical-map"} {
		payload := append([]byte(nil), base.Armor.Data...)
		switch name {
		case "truncated":
			payload = payload[:len(payload)-1]
		case "trailing-byte":
			payload = append(payload, 0)
		case "invalid-enum":
			payload[0] = 255
		case "noncanonical-map":
			genesis, _ := base.Value.Field("genesis")
			globals, _ := genesis.Field("globals")
			globals = globals.Unwrap()
			if len(globals.Entries) < 2 {
				return fmt.Errorf("fixture needs two global map entries")
			}
			var reordered bytes.Buffer
			reordered.WriteByte(byte(len(globals.Entries)))
			order := make([]int, len(globals.Entries))
			for i := range order {
				order[i] = i
			}
			order[0], order[1] = 1, 0
			for _, index := range order {
				reordered.Write(globals.Entries[index].Key.Encoded)
				reordered.Write(globals.Entries[index].Value.Encoded)
			}
			position := bytes.Index(payload, globals.Encoded)
			if position < 0 || reordered.Len() != len(globals.Encoded) {
				return fmt.Errorf("locate canonical globals")
			}
			copy(payload[position:], reordered.Bytes())
		}
		_, err := consignment.Decode(payload)
		if err == nil {
			return fmt.Errorf("Go accepted malformed %s", name)
		}
		item := validationCase{Name: name, Binary: true, Path: filepath.Join(dir, name+".bin"), Evidence: filepath.Join(dir, name+".json"), Expected: "invalid", GoOutcome: "invalid", GoError: err.Error()}
		if err := os.WriteFile(item.Path, payload, 0o600); err != nil {
			return err
		}
		raw, _ := json.Marshal(evidence)
		if err := os.WriteFile(item.Evidence, raw, 0o600); err != nil {
			return err
		}
		cases = append(cases, item)
	}
	// Also validate the original Go->Rust transfer gate itself, byte-identical
	// to the upstream fixture, with the fixture's actual Bitcoin witness.
	officialRaw, err := os.ReadFile("testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		return err
	}
	witnessRaw, err := hex.DecodeString("0200000001c568200c10c4ca3c351108bffc3d1e4238f94d94c06d28b6cd91a1b15b5d29140100000000fdffffff020000000000000000226a208bef6db012dbd42088e5af8ac1df536ff8de140e82fe34a0bbb3e13b912b55b322020000000000000000000000")
	if err != nil {
		return err
	}
	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(witnessRaw)); err != nil {
		return err
	}
	evidence.Witnesses = []witnessFact{{TxID: tx.TxHash().String(), Raw: hex.EncodeToString(witnessRaw), State: "tentative", Height: 800000, Timestamp: 1700000100}}
	evidence.prev = consignment.Outpoint{TxID: [32]byte(tx.TxIn[0].PreviousOutPoint.Hash), Vout: tx.TxIn[0].PreviousOutPoint.Index}
	if err := add("nia-official-transfer", string(officialRaw), evidence, "valid"); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(cases, "", "  ")
	return os.WriteFile(filepath.Join(dir, "cases.json"), raw, 0o600)
}

func buildValidationTransfer(kind, mutation string) (string, validationEvidence, error) {
	data, err := os.ReadFile("testvectors/rc11/" + kind + "-example.rgba")
	if err != nil {
		return "", validationEvidence{}, err
	}
	base, err := consignment.DecodeArmor(string(data))
	if err != nil {
		return "", validationEvidence{}, err
	}
	genesis, _ := base.Value.Field("genesis")
	gc, err := operations.CommitGenesis(genesis)
	if err != nil {
		return "", validationEvidence{}, err
	}
	outputs, err := schemas.RevealedOutputs(genesis)
	if err != nil {
		return "", validationEvidence{}, err
	}
	var owner schemas.RevealedOutput
	ty := uint64(4000)
	inflate := mutation == "inflate-valid" || mutation == "inflate-excess" || mutation == "inflate-no-right"
	if inflate && mutation != "inflate-no-right" {
		ty = 4010
	}
	found := false
	for _, o := range outputs {
		if o.AssignmentType == ty && o.Index == 0 {
			owner = o
			found = true
			break
		}
	}
	if !found {
		return "", validationEvidence{}, fmt.Errorf("%s fixture has no assignment %d", kind, ty)
	}
	seal, err := seals.DecodeGraphBlindSeal(append([]byte{1}, owner.Seal.Unwrap().Encoded...))
	if err != nil {
		return "", validationEvidence{}, fmt.Errorf("%s seal %x: %w", kind, owner.Seal.Unwrap().Encoded, err)
	}
	if seal.TxID == nil {
		return "", validationEvidence{}, fmt.Errorf("genesis seal has no txid")
	}
	prev := consignment.Outpoint{TxID: *seal.TxID, Vout: seal.Vout}
	input := operations.TransitionInput{OperationID: gc.OperationID, AssignmentType: uint16(ty), Index: 0}
	if mutation == "missing-history" {
		input.OperationID[0] ^= 1
	}
	destination := seals.NewWitnessBlindSeal(1, 0x0102030405060708)
	spec := operations.TransitionSpec{ContractID: gc.OperationID, Nonce: 42, TransitionType: 10000, Inputs: []operations.TransitionInput{input}, Outputs: []operations.TransitionOutput{{AssignmentType: 4000, Class: owner.State.Class, Amount: owner.State.Amount, Data: append([]byte(nil), owner.State.Data...), RevealedSeal: &destination}}}
	if mutation == "invalid-state" {
		if kind == "uda" {
			spec.Outputs[0].Data[0] ^= 1
		} else {
			spec.Outputs[0].Amount++
		}
	}
	if inflate {
		remaining := owner.State.Amount - 1
		if mutation == "inflate-excess" {
			remaining++
		}
		amt := func(n uint64) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, n); return b }
		spec.TransitionType = 8000
		spec.Metadata = map[uint16][]byte{1000: amt(remaining)}
		spec.Globals = map[uint16][][]byte{2010: {amt(1)}}
		second := seals.NewWitnessBlindSeal(1, 0x0102030405060709)
		spec.Outputs = []operations.TransitionOutput{{AssignmentType: 4000, Class: "fungible", Amount: 1, RevealedSeal: &destination}, {AssignmentType: 4010, Class: "fungible", Amount: remaining, RevealedSeal: &second}}
	}
	transition, _, err := operations.BuildTransition(spec)
	if err != nil {
		return "", validationEvidence{}, err
	}
	bundle, bc, err := operations.BuildBundle(spec.Inputs, transition)
	if err != nil {
		return "", validationEvidence{}, err
	}
	proof, err := anchors.NewMPCProofFrom(gc.OperationID, 3, bytes.NewReader(bytes.Repeat([]byte{0x42}, 96)))
	if err != nil {
		return "", validationEvidence{}, err
	}
	commitment, err := anchors.ConvolveMPC(proof, gc.OperationID, bc.BundleID)
	if err != nil {
		return "", validationEvidence{}, err
	}
	tx := wire.NewMsgTx(2)
	point := wire.OutPoint{Hash: chainhash.Hash(prev.TxID), Index: prev.Vout}
	if mutation == "seal-mismatch" {
		point.Index++
	}
	tx.AddTxIn(wire.NewTxIn(&point, nil, nil))
	script := append([]byte{0x6a, 0x20}, commitment[:]...)
	if mutation == "anchor-mismatch" {
		script[2] ^= 1
	}
	tx.AddTxOut(wire.NewTxOut(0, script))
	tx.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))
	wb, err := operations.BuildOpretWitnessBundleWithTx(tx, bundle, proof)
	if err != nil {
		return "", validationEvidence{}, err
	}
	secret, err := destination.Conceal()
	if err != nil {
		return "", validationEvidence{}, err
	}
	built, err := consignment.BuildTransfer(base, wb, [][32]byte{[32]byte(secret)})
	if err != nil {
		return "", validationEvidence{}, err
	}
	armor, err := consignment.EncodeArmor(built.Value)
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	network, _ := genesis.Field("chainNet")
	chain := []string{"bc", "tb", "tb4", "sb", "bcrt"}[network.Unwrap().Tag]
	e := validationEvidence{Chain: chain, prev: prev, Witnesses: []witnessFact{{TxID: tx.TxHash().String(), Raw: hex.EncodeToString(raw.Bytes()), State: "tentative", Height: 800000, Timestamp: 1700000100}}}
	return armor, e, err
}
