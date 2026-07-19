package consignment

import (
	"bytes"
	"testing"

	"github.com/sat20-labs/rgb11/operations"
)

func TestAppendOperationGlobalsAdvancesHistoryContext(t *testing.T) {
	var contractID, inputID, secret [32]byte
	contractID[0], inputID[0], secret[0] = 1, 2, 3
	transition, _, err := operations.BuildTransition(operations.TransitionSpec{
		ContractID: contractID, TransitionType: 8000,
		Globals: map[uint16][][]byte{2010: {{10}, {20}}},
		Inputs:  []operations.TransitionInput{{OperationID: inputID, AssignmentType: 4010}},
		Outputs: []operations.TransitionOutput{{AssignmentType: 4010, Class: "fungible", Amount: 1, SecretSeal: secret}},
	})
	if err != nil {
		t.Fatal(err)
	}
	context := map[uint64][][]byte{2010: {{1}}}
	if err := appendOperationGlobals(context, transition); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{1}, {10}, {20}}
	if len(context[2010]) != len(want) {
		t.Fatalf("global history length %d, want %d", len(context[2010]), len(want))
	}
	for index := range want {
		if !bytes.Equal(context[2010][index], want[index]) {
			t.Fatalf("global history item %d = %x, want %x", index, context[2010][index], want[index])
		}
	}
}
