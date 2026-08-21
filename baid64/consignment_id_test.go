package baid64

import "testing"

func TestConsignmentIDCanonicalRoundTrip(t *testing.T) {
	var payload [32]byte
	payload[0] = 0x11
	payload[31] = 0xa4
	encoded, err := Encode32(payload, ConsignmentIDOptions())
	if err != nil {
		t.Fatalf("encode consignment ID: %v", err)
	}
	decoded, err := Decode32(encoded, ConsignmentIDOptions())
	if err != nil {
		t.Fatalf("decode consignment ID: %v", err)
	}
	if decoded != payload {
		t.Fatalf("decoded payload=%x, want %x", decoded, payload)
	}
	canonical, err := Encode32(decoded, ConsignmentIDOptions())
	if err != nil {
		t.Fatalf("re-encode consignment ID: %v", err)
	}
	if canonical != encoded {
		t.Fatalf("canonical ID=%q, want %q", canonical, encoded)
	}
}
