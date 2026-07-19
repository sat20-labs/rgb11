package consensus

import (
	"crypto/sha256"
	"testing"
)

func TestTaggedHash(t *testing.T) {
	tag := "urn:lnp-bp:rgb:operation#2024-02-03"
	data := []byte{1, 2, 3}
	tagHash := sha256.Sum256([]byte(tag))
	expected := sha256.Sum256(append(append(append([]byte{}, tagHash[:]...), tagHash[:]...), data...))
	if got := TaggedHash(tag, data); got != expected {
		t.Fatalf("tagged hash mismatch %x != %x", got, expected)
	}
}
