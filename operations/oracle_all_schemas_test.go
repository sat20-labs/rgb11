package operations_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
)

func TestBuildEveryWalletSchemaTransferMatchesRust(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/core.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Vectors map[string]string `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}

	secretSeal, err := seals.NewWitnessBlindSeal(1, 0x0102030405060708).Conceal()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nia", "ifa", "cfa", "uda"} {
		t.Run(name, func(t *testing.T) {
			armored, err := os.ReadFile("../testvectors/rc11/" + name + "-example.rgba")
			if err != nil {
				t.Fatal(err)
			}
			container, err := consignment.DecodeArmor(string(armored))
			if err != nil {
				t.Fatal(err)
			}
			genesis, ok := container.Value.Field("genesis")
			if !ok {
				t.Fatal("official contract has no genesis")
			}
			commitment, err := operations.CommitGenesis(genesis)
			if err != nil {
				t.Fatal(err)
			}
			outputs, err := schemas.RevealedOutputs(genesis)
			if err != nil {
				t.Fatal(err)
			}
			var owner *schemas.RevealedOutput
			for index := range outputs {
				if outputs[index].AssignmentType == 4000 && outputs[index].Index == 0 {
					owner = &outputs[index]
					break
				}
			}
			if owner == nil {
				t.Fatal("official contract has no revealed owner allocation")
			}

			transition, transitionCommitment, err := operations.BuildTransition(operations.TransitionSpec{
				ContractID:     commitment.OperationID,
				Nonce:          42,
				TransitionType: 10000,
				Inputs: []operations.TransitionInput{{
					OperationID: commitment.OperationID, AssignmentType: 4000, Index: 0,
				}},
				Outputs: []operations.TransitionOutput{{
					AssignmentType: 4000,
					Class:          owner.State.Class,
					Amount:         owner.State.Amount,
					Data:           append([]byte(nil), owner.State.Data...),
					SecretSeal:     [32]byte(secretSeal),
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := hex.EncodeToString(transition.Encoded), corpus.Vectors[name+"_confidential_transition_strict_hex"]; got != want {
				t.Fatalf("strict transition mismatch\n got %s\nwant %s", got, want)
			}
			if got, want := hex.EncodeToString(transitionCommitment.OperationID[:]), corpus.Vectors[name+"_confidential_transition_id"]; got != want {
				t.Fatalf("transition id mismatch: got %s want %s", got, want)
			}
		})
	}
}
