package operations_test

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/operations"
)

func TestOfficialNIABundleCommitment(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := consignment.DecodeArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	bundles, ok := container.Value.Field("bundles")
	if !ok || len(bundles.Items) != 1 {
		t.Fatalf("unexpected bundle count")
	}
	bundle, ok := bundles.Items[0].Field("bundle")
	if !ok {
		t.Fatal("bundle missing")
	}
	report, err := operations.CommitBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(report.BundleID[:]); got != "3696f9eeba53cd9727f604e690c4b7f1b0f2b2be239276b903e6ae385855013e" {
		t.Fatalf("bundle id mismatch: %s", got)
	}
	if len(report.Transitions) != 1 {
		t.Fatalf("unexpected known transition count %d", len(report.Transitions))
	}
}
