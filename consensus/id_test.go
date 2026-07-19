package consensus

import "testing"

func TestContractIDCanonicalString(t *testing.T) {
	canonical := "rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E"
	id, err := ParseContractID(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if got := id.String(); got != canonical {
		t.Fatalf("contract ID %q != %q", got, canonical)
	}
}

func TestSecretSealCanonicalString(t *testing.T) {
	canonical := "utxob:xDfmDF9g-yNOjriV-6Anbe6H-MLJ__g6-lo7Dd4f-dhWBW8S-XYGBm"
	id, err := ParseSecretSeal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if got := id.String(); got != canonical {
		t.Fatalf("secret seal %q != %q", got, canonical)
	}
}
