package schemas_test

import (
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

func TestValidateOfficialNiaTransitionVector(t *testing.T) {
	const transitionHex = "0000934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec2a00000000000000102700000100934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936eca00f00000100a00f010100000001000000080706050403020108a08601000000000000"
	armored, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := consignment.DecodeArmor(string(armored))
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := container.Value.Field("schema")
	types, _ := container.Value.Field("types")
	encoded, _ := hex.DecodeString(transitionHex)
	registry, _ := strict_types.RC11Registry()
	transition, err := registry.Decode("RGBCommit", "Transition", encoded)
	if err != nil {
		t.Fatal(err)
	}
	contractID, err := baid64.Decode32(container.ContractID, baid64.RGBContractOptions())
	if err != nil {
		t.Fatal(err)
	}
	resolver := schemas.InputResolverFunc(func(ref schemas.InputRef) (schemas.ResolvedInput, error) {
		if ref.AssignmentType != 4000 || ref.Index != 0 {
			return schemas.ResolvedInput{}, errors.New("unexpected input")
		}
		return schemas.ResolvedInput{Class: "fungible", Amount: 100000}, nil
	})
	report, err := schemas.ValidateTransition(schema, types, transition, schemas.TransitionContext{ContractID: contractID}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConformanceValid || !report.StateTypesValid || !report.InputsResolved || !report.ScriptValid ||
		report.InputAmounts[4000] != 100000 || report.OutputAmounts[4000] != 100000 {
		t.Fatalf("unexpected transition report: %+v", report)
	}
}

func TestNiaTransitionRejectsUnbalancedResolvedInput(t *testing.T) {
	const transitionHex = "0000934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936ec2a00000000000000102700000100934bec6bace308b61f9ebbbadee9ee26fa99d88549e7394437394532488936eca00f00000100a00f010100000001000000080706050403020108a08601000000000000"
	armored, _ := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	container, _ := consignment.DecodeArmor(string(armored))
	schema, _ := container.Value.Field("schema")
	types, _ := container.Value.Field("types")
	encoded, _ := hex.DecodeString(transitionHex)
	registry, _ := strict_types.RC11Registry()
	transition, _ := registry.Decode("RGBCommit", "Transition", encoded)
	contractID, _ := baid64.Decode32(container.ContractID, baid64.RGBContractOptions())
	resolver := schemas.InputResolverFunc(func(schemas.InputRef) (schemas.ResolvedInput, error) {
		return schemas.ResolvedInput{Class: "fungible", Amount: 99999}, nil
	})
	if _, err := schemas.ValidateTransition(schema, types, transition, schemas.TransitionContext{ContractID: contractID}, resolver); err == nil {
		t.Fatal("unbalanced transition accepted")
	}
}
