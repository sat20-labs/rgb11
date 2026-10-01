package consignment

import (
	"errors"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
	"testing"
)

func TestSideBranchBounds(t *testing.T) {
	genesis, first, last, other := [32]byte{1}, [32]byte{2}, [32]byte{3}, [32]byte{4}
	oldBundle, endBundle, otherBundle := [32]byte{11}, [32]byte{12}, [32]byte{13}
	for _, tc := range []struct {
		name                                   string
		transfer, complete                     bool
		terminals                              [][32]byte
		multiTip, sameBundle, wantOld, wantErr bool
	}{
		{name: "unique_tip", transfer: true, complete: true, wantOld: true},
		{name: "undisclosed_operation", transfer: true},
		{name: "multiple_tips", transfer: true, complete: true, multiTip: true},
		{name: "contract", complete: true},
		{name: "explicit_terminal", transfer: true, complete: true, terminals: [][32]byte{endBundle}, wantOld: true},
		{name: "explicit_unrelated_tip", transfer: true, complete: true, terminals: [][32]byte{endBundle}, multiTip: true, wantOld: true},
		{name: "protect_old_bundle", transfer: true, complete: true, terminals: [][32]byte{oldBundle}},
		{name: "protect_both", transfer: true, complete: true, terminals: [][32]byte{oldBundle, endBundle}},
		{name: "same_bundle", transfer: true, complete: true, sameBundle: true},
		{name: "missing_binding", transfer: true, complete: true, terminals: [][32]byte{{99}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := map[[32]byte]validatedOp{genesis: {}, first: {bundle: oldBundle, parents: [][32]byte{genesis}}, last: {bundle: endBundle, parents: [][32]byte{first}}}
			bundles := map[[32]byte][][32]byte{oldBundle: {first}, endBundle: {last}}
			if tc.multiTip {
				graph[other] = validatedOp{bundle: otherBundle, parents: [][32]byte{genesis}}
				bundles[otherBundle] = [][32]byte{other}
			}
			if tc.sameBundle {
				graph[first] = validatedOp{bundle: endBundle, parents: [][32]byte{genesis}}
				delete(bundles, oldBundle)
				bundles[endBundle] = [][32]byte{first, last}
			}
			terminals := strict_types.Value{Kind: strict_types.ValueMap}
			for _, id := range tc.terminals {
				terminals.Entries = append(terminals.Entries, strict_types.Entry{Key: strict_types.Value{Kind: strict_types.ValueBytes, Raw: id[:]}})
			}
			transfer := "false"
			if tc.transfer {
				transfer = "true"
			}
			value := strict_types.Value{Kind: strict_types.ValueStruct, Fields: []strict_types.Field{{Name: "transfer", Value: strict_types.Value{Kind: strict_types.ValueEnum, Name: transfer}}, {Name: "terminals", Value: terminals}}}
			historical, err := historicalSideOps(value, graph, bundles, tc.complete)
			if tc.wantErr {
				if !errors.Is(err, operations.ErrInvalidBundle) {
					t.Fatalf("unbound terminal must reject, got %v", err)
				}
				return
			}
			if err != nil || historical[first] != tc.wantOld {
				t.Fatalf("historical ancestor=%v want=%v error=%v", historical[first], tc.wantOld, err)
			}
			if historical[last] || historical[other] {
				t.Fatal("terminal or unrelated branch classified as historical ancestor")
			}
		})
	}
}

func TestTerminalBundleBinding(t *testing.T) {
	container, resolver := loadValidationVector(t)
	terminals := unwrapPointer(fieldPointer(&container.Value, "terminals"))
	// The witness vector has no terminal secrets; inject an absent bundle
	// binding to verify that full validation cannot trust a declared target.
	terminals.Entries = []strict_types.Entry{{
		Key: strict_types.Value{Kind: strict_types.ValueBytes, Raw: make([]byte, 32)},
	}}
	if _, err := container.Validate(resolver); !errors.Is(err, operations.ErrInvalidBundle) {
		t.Fatalf("absent terminal bundle must reject, got %v", err)
	}
}

type sideResolver struct {
	vectorResolver
	evidence OutpointEvidence
	err      error
}

func (r sideResolver) ResolveRGB11Outpoint(Outpoint) (OutpointEvidence, error) {
	return r.evidence, r.err
}

func TestSideBranchEvidence(t *testing.T) {
	for _, tc := range []struct {
		name             string
		evidence         OutpointEvidence
		resolverErr      error
		historical, keep bool
		want             error
	}{
		{name: "spent_ancestor", evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, historical: true},
		{name: "unspent_ancestor", evidence: OutpointEvidence{Known: true, Exists: true}, historical: true, keep: true},
		{name: "spent_terminal", evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, want: ErrOutpointSpend},
		{name: "unknown_ancestor", evidence: OutpointEvidence{Exists: true, Spent: true}, historical: true, want: ErrOutpointUnknown},
		{name: "missing_ancestor", evidence: OutpointEvidence{Known: true, Spent: true}, historical: true, want: ErrOutpointUnknown},
		{name: "resolver_error", evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, resolverErr: errors.New("offline"), historical: true, want: ErrOutpointUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keep, err := verifyProjectedOutpoint(sideResolver{evidence: tc.evidence, err: tc.resolverErr}, Outpoint{}, tc.historical)
			if keep != tc.keep || !errors.Is(err, tc.want) {
				t.Fatalf("keep=%v error=%v; want keep=%v error=%v", keep, err, tc.keep, tc.want)
			}
		})
	}
}
