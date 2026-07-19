package issuance_test

import (
	"bytes"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/issuance"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
)

type existingOutputs struct{}

func (existingOutputs) ResolveRGB11Witness([32]byte) (consignment.WitnessEvidence, error) {
	return consignment.WitnessEvidence{}, nil
}

func (existingOutputs) ResolveRGB11Outpoint(consignment.Outpoint) (consignment.OutpointEvidence, error) {
	return consignment.OutpointEvidence{Known: true, Exists: true}, nil
}

func TestIssueFirstReleaseSchemas(t *testing.T) {
	txidA := bytes.Repeat([]byte{0x11}, 32)
	txidB := bytes.Repeat([]byte{0x22}, 32)
	txidC := bytes.Repeat([]byte{0x33}, 32)
	sealA, _ := seals.NewGraphBlindSeal(txidA, 0, 1)
	sealB, _ := seals.NewGraphBlindSeal(txidB, 1, 2)
	sealC, _ := seals.NewGraphBlindSeal(txidC, 2, 3)

	tests := []issuance.Spec{
		{
			Kind: schemas.NIA, Network: issuance.BitcoinTestnet4,
			Ticker: "SAT", Name: "SAT20 NIA", Details: "non-inflatable test asset", Precision: 8,
			Allocations: []issuance.Allocation{{Seal: sealB, Amount: 40}, {Seal: sealA, Amount: 60}}, Timestamp: 1_700_000_001,
		},
		{
			Kind: schemas.IFA, Network: issuance.BitcoinTestnet4,
			Ticker: "SIF", Name: "SAT20 IFA", Precision: 2,
			Allocations:     []issuance.Allocation{{Seal: sealA, Amount: 100}},
			InflationRights: []issuance.Allocation{{Seal: sealC, Amount: 900}},
			RejectListURL:   "https://example.com/reject.txt", Timestamp: 1_700_000_002,
		},
		{
			Kind: schemas.UDA, Network: issuance.BitcoinTestnet3,
			Ticker: "SUDA", Name: "SAT20 Unique", Details: "unique token", Precision: 0,
			Allocations: []issuance.Allocation{{Seal: sealA, Amount: 1}}, Timestamp: 1_700_000_004,
		},
	}

	seen := make(map[string]struct{})
	for _, spec := range tests {
		t.Run(string(spec.Kind), func(t *testing.T) {
			issued, err := issuance.Issue(spec)
			if err != nil {
				t.Fatal(err)
			}
			if issued.ContractID == "" || issued.SchemaID == "" || issued.Container == nil || issued.Armor == "" {
				t.Fatalf("incomplete issuance result: %+v", issued)
			}
			if _, duplicate := seen[issued.ContractID]; duplicate {
				t.Fatalf("duplicate contract id %s", issued.ContractID)
			}
			seen[issued.ContractID] = struct{}{}
			report, err := issued.Container.Validate(existingOutputs{})
			if err != nil {
				t.Fatal(err)
			}
			if !report.ConsensusValid || len(report.CurrentStates) != len(spec.Allocations)+len(spec.InflationRights) {
				t.Fatalf("unexpected validation report: %+v", report)
			}
		})
	}
}

func TestIssueRejectsCFAInFirstRelease(t *testing.T) {
	txid := bytes.Repeat([]byte{0x44}, 32)
	seal, err := seals.NewGraphBlindSeal(txid, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = issuance.Issue(issuance.Spec{
		Kind: schemas.CFA, Network: issuance.BitcoinTestnet4,
		Name: "SAT20 CFA", Allocations: []issuance.Allocation{{Seal: seal, Amount: 1}},
	})
	if err != issuance.ErrUnsupportedSchema {
		t.Fatalf("CFA issuance error=%v, want %v", err, issuance.ErrUnsupportedSchema)
	}
}
