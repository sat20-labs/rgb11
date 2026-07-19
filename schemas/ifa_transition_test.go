package schemas_test

import (
	"encoding/binary"
	"testing"

	"github.com/sat20-labs/rgb11/issuance"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
)

func TestIFAInflationAndBurnRules(t *testing.T) {
	var txidA, txidB [32]byte
	txidA[0], txidB[0] = 1, 2
	assetSeal, _ := seals.NewGraphBlindSeal(txidA[:], 0, 1)
	inflationSeal, _ := seals.NewGraphBlindSeal(txidB[:], 0, 2)
	issued, err := issuance.Issue(issuance.Spec{
		Kind: schemas.IFA, Network: issuance.BitcoinTestnet4,
		Ticker: "SIFA", Name: "SAT20 IFA", Precision: 2,
		Allocations:     []issuance.Allocation{{Seal: assetSeal, Amount: 100}},
		InflationRights: []issuance.Allocation{{Seal: inflationSeal, Amount: 900}},
		Timestamp:       1_700_001_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := issued.Container.Value.Field("schema")
	types, _ := issued.Container.Value.Field("types")
	genesis, _ := issued.Container.Value.Field("genesis")
	genesisCommitment, err := operations.CommitGenesis(genesis)
	if err != nil {
		t.Fatal(err)
	}
	context := schemas.TransitionContext{ContractID: genesisCommitment.OperationID}

	var assetSecret, inflationSecret [32]byte
	assetSecret[0], inflationSecret[0] = 3, 4
	inflate, _, err := operations.BuildTransition(operations.TransitionSpec{
		ContractID: genesisCommitment.OperationID, Nonce: 1, TransitionType: 8000,
		Metadata: map[uint16][]byte{1000: amountBytes(800)},
		Globals:  map[uint16][][]byte{2010: {amountBytes(100)}},
		Inputs:   []operations.TransitionInput{{OperationID: genesisCommitment.OperationID, AssignmentType: 4010, Index: 0}},
		Outputs: []operations.TransitionOutput{
			{AssignmentType: 4000, Class: "fungible", Amount: 100, SecretSeal: assetSecret},
			{AssignmentType: 4010, Class: "fungible", Amount: 800, SecretSeal: inflationSecret},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := schemas.ValidateTransition(schema, types, inflate, context,
		schemas.InputResolverFunc(func(ref schemas.InputRef) (schemas.ResolvedInput, error) {
			return schemas.ResolvedInput{Class: "fungible", Amount: 900}, nil
		}))
	if err != nil || !report.ScriptValid || report.OutputAmounts[4000] != 100 || report.OutputAmounts[4010] != 800 {
		t.Fatalf("inflate report=%+v err=%v", report, err)
	}

	burn, _, err := operations.BuildTransition(operations.TransitionSpec{
		ContractID: genesisCommitment.OperationID, Nonce: 2, TransitionType: 8010,
		Metadata: map[uint16][]byte{1001: amountBytes(10), 1002: amountBytes(50)},
		Inputs: []operations.TransitionInput{
			{OperationID: genesisCommitment.OperationID, AssignmentType: 4000, Index: 0},
			{OperationID: genesisCommitment.OperationID, AssignmentType: 4010, Index: 0},
		},
		Outputs: []operations.TransitionOutput{
			{AssignmentType: 4000, Class: "fungible", Amount: 90, SecretSeal: assetSecret},
			{AssignmentType: 4010, Class: "fungible", Amount: 850, SecretSeal: inflationSecret},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err = schemas.ValidateTransition(schema, types, burn, context,
		schemas.InputResolverFunc(func(ref schemas.InputRef) (schemas.ResolvedInput, error) {
			if ref.AssignmentType == 4000 {
				return schemas.ResolvedInput{Class: "fungible", Amount: 100}, nil
			}
			return schemas.ResolvedInput{Class: "fungible", Amount: 900}, nil
		}))
	if err != nil || !report.ScriptValid || report.OutputAmounts[4000] != 90 || report.OutputAmounts[4010] != 850 {
		t.Fatalf("burn report=%+v err=%v", report, err)
	}
}

func amountBytes(value uint64) []byte {
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint64(raw, value)
	return raw
}
