package consignment

import (
	"errors"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/strict_types"
)

func TestContractSpentGenesis(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		evidence           OutpointEvidence
		resolverErr        error
		transfer, terminal bool
		want               error
		states             int
	}{
		{name: "unspent", evidence: OutpointEvidence{Known: true, Exists: true}, states: 1},
		{name: "spent", evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}},
		{name: "unknown", evidence: OutpointEvidence{Exists: true, Spent: true}, want: ErrOutpointUnknown},
		{name: "missing", evidence: OutpointEvidence{Known: true, Spent: true}, want: ErrOutpointUnknown},
		{name: "resolver_error", evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, resolverErr: errors.New("offline"), want: ErrOutpointUnknown},
		{name: "transfer", transfer: true, evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, want: ErrOutpointSpend},
		{name: "terminal", terminal: true, evidence: OutpointEvidence{Known: true, Exists: true, Spent: true}, want: ErrOutpointSpend},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, e := DecodeArmor(string(raw))
			if e != nil {
				t.Fatal(e)
			}
			if tc.transfer {
				unwrapPointer(fieldPointer(&c.Value, "transfer")).Name = "true"
			}
			if tc.terminal {
				unwrapPointer(fieldPointer(&c.Value, "terminals")).Entries = []strict_types.Entry{{
					Key: strict_types.Value{Kind: strict_types.ValueBytes, Raw: make([]byte, 32)},
				}}
			}
			v, e := c.Validate(sideResolver{evidence: tc.evidence, err: tc.resolverErr})
			if !errors.Is(e, tc.want) {
				t.Fatalf("error=%v, want=%v", e, tc.want)
			}
			if e == nil && (!v.ConsensusValid || len(v.CurrentStates) != tc.states) {
				t.Fatalf("valid=%v states=%d, want %d", v.ConsensusValid, len(v.CurrentStates), tc.states)
			}
		})
	}
}

func TestContractBundleStaysStrict(t *testing.T) {
	c, resolver := loadValidationVector(t)
	unwrapPointer(fieldPointer(&c.Value, "transfer")).Name = "false"
	resolver.currentSpent = true
	if _, err := c.Validate(resolver); !errors.Is(err, ErrOutpointSpend) {
		t.Fatalf("contract containing bundles must retain spent-output rejection, got %v", err)
	}
}
