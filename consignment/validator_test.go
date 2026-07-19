package consignment

import (
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

type vectorResolver struct {
	raw      []byte
	txid     [32]byte
	prev     Outpoint
	archived bool
	unknown  bool
}

func (r vectorResolver) ResolveRGB11Witness(txid [32]byte) (WitnessEvidence, error) {
	if txid != r.txid {
		return WitnessEvidence{}, errors.New("unexpected witness")
	}
	state := WitnessTentative
	if r.archived {
		state = WitnessArchived
	}
	return WitnessEvidence{RawTx: r.raw, State: state}, nil
}

func (r vectorResolver) ResolveRGB11Outpoint(outpoint Outpoint) (OutpointEvidence, error) {
	if r.unknown {
		return OutpointEvidence{}, nil
	}
	if outpoint == r.prev {
		spending := r.txid
		return OutpointEvidence{Known: true, Exists: true, Spent: true, SpendingTxID: &spending}, nil
	}
	if outpoint.TxID == r.txid {
		return OutpointEvidence{Known: true, Exists: true}, nil
	}
	return OutpointEvidence{}, nil
}

func TestValidateOfficialNIATransferEndToEnd(t *testing.T) {
	container, resolver := loadValidationVector(t)
	report, err := container.Validate(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConsensusValid || !container.ConsensusValid || report.Bundles != 1 || report.Transitions != 1 ||
		report.Anchors != 1 || report.PendingWitnesses != 1 || len(report.CurrentStates) != 1 {
		t.Fatalf("unexpected validation report: %+v", report)
	}
	if state := report.CurrentStates[0]; state.State.Class != "fungible" || state.State.Amount != 100000 ||
		state.Outpoint.TxID != resolver.txid || state.CarrierBinding.CommitmentMethod != "opret1st" {
		t.Fatalf("unexpected projected state: %+v", state)
	}
}

func TestValidateRejectsReorgedWitness(t *testing.T) {
	container, resolver := loadValidationVector(t)
	resolver.archived = true
	if _, err := container.Validate(resolver); !errors.Is(err, ErrWitnessArchived) {
		t.Fatalf("expected archived witness rejection, got %v", err)
	}
}

func TestValidateRejectsUnknownSpendState(t *testing.T) {
	container, resolver := loadValidationVector(t)
	resolver.unknown = true
	if _, err := container.Validate(resolver); !errors.Is(err, ErrOutpointUnknown) {
		t.Fatalf("expected unknown outpoint rejection, got %v", err)
	}
}

func loadValidationVector(t *testing.T) (*Container, vectorResolver) {
	t.Helper()
	armored, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := DecodeArmor(string(armored))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString("0200000001c568200c10c4ca3c351108bffc3d1e4238f94d94c06d28b6cd91a1b15b5d29140100000000fdffffff020000000000000000226a208bef6db012dbd42088e5af8ac1df536ff8de140e82fe34a0bbb3e13b912b55b322020000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	bundles, _ := container.Value.Field("bundles")
	publicWitness, _ := bundles.Items[0].Field("pubWitness")
	tx, err := txFromStrictValue(publicWitness.Unwrap().Inner.Unwrap())
	if err != nil {
		t.Fatal(err)
	}
	txid := hashArray(tx.TxHash())
	prev := Outpoint{TxID: [32]byte(tx.TxIn[0].PreviousOutPoint.Hash), Vout: tx.TxIn[0].PreviousOutPoint.Index}
	return container, vectorResolver{raw: raw, txid: txid, prev: prev}
}
