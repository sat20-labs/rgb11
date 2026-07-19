package sync

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestWalletHeadChainAndLatest(t *testing.T) {
	first := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 1, StateHash: [32]byte{1}, OperationID: [32]byte{2}}
	if err := first.ValidateSuccessor(nil); err != nil {
		t.Fatal(err)
	}
	second := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 2, StateHash: [32]byte{3}, OperationID: [32]byte{4}}
	if err := second.ValidateSuccessor(&first); err != nil {
		t.Fatal(err)
	}
	latest, err := SelectLatest([]WalletHead{first, second}, "wallet-a")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Seq != 2 {
		t.Fatalf("latest = %#v", latest)
	}
}

func TestWalletHeadRejectsWrongWalletAndSequence(t *testing.T) {
	head := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 1, StateHash: [32]byte{1}}
	head.OperationID = [32]byte{2}
	if !errors.Is(head.Validate("wallet-b"), ErrHeadWallet) {
		t.Fatal("wrong wallet accepted")
	}
	bad := head
	bad.Seq = 2
	if !errors.Is(bad.ValidateSuccessor(nil), ErrHeadSequence) {
		t.Fatal("invalid first sequence accepted")
	}
	bad = head
	bad.OperationID = [32]byte{}
	if !errors.Is(bad.Validate("wallet-a"), ErrHeadField) {
		t.Fatal("zero operation id accepted")
	}
}

func TestWalletHeadDetectsSameSequenceConflict(t *testing.T) {
	left := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 1, StateHash: [32]byte{1}, OperationID: [32]byte{3}}
	right := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 1, StateHash: [32]byte{2}, OperationID: [32]byte{4}}
	if _, err := SelectLatest([]WalletHead{left, right}, "wallet-a"); !errors.Is(err, ErrHeadConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestWalletHeadPayloadContainsOnlyRestoreAndOrderingFields(t *testing.T) {
	head := WalletHead{Version: HeadVersion, WalletID: "wallet-a", Seq: 1, StateHash: [32]byte{1}, OperationID: [32]byte{2}}
	encoded, err := json.Marshal(head)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "wallet_id", "seq", "state_hash", "operation_id"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("missing head field %q", name)
		}
	}
	if len(fields) != 5 {
		t.Fatalf("wallet head contains nonessential fields: %s", encoded)
	}
}
