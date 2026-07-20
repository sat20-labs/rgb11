package relay

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
)

type edSigner ed25519.PrivateKey

func (s edSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(s), message), nil
}

func verifyED(pubKey, message, signature []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(pubKey), message, signature)
}

func keyPair(t *testing.T) (ed25519.PublicKey, edSigner) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, edSigner(priv)
}

func TestRelayAndAckUseDifferentSignersAndKeys(t *testing.T) {
	relayKey, ackKey, err := NewTemporaryKeys()
	if err != nil {
		t.Fatal(err)
	}
	if relayKey == ackKey {
		t.Fatal("relay and ack keys must differ")
	}
	if err := ValidateTemporaryKey(relayKey); err != nil {
		t.Fatal(err)
	}
	senderPub, sender := keyPair(t)
	receiverPub, receiver := keyPair(t)
	record := RelayRecord{
		Version: RecordVersion, TransferID: "transfer-1", RecipientID: "recipient-1",
		ObjectHash: [32]byte{1}, ObjectSize: 42, SourcePeerID: "peer-1",
		AckRecordKey: ackKey, Expiry: 200, SenderPubKey: senderPub,
	}
	if err := record.Sign(sender); err != nil {
		t.Fatal(err)
	}
	if err := record.Verify(senderPub, 100, verifyED); err != nil {
		t.Fatal(err)
	}
	relayHash, err := record.Hash()
	if err != nil {
		t.Fatal(err)
	}
	ack := AckRecord{
		Version: RecordVersion, TransferID: record.TransferID, RecipientID: record.RecipientID,
		RelayRecordHash: relayHash, ConsignmentHash: record.ObjectHash, Accepted: true,
		RecipientPubKey: receiverPub,
	}
	if err := ack.Sign(receiver); err != nil {
		t.Fatal(err)
	}
	if err := ack.Verify(receiverPub, verifyED); err != nil {
		t.Fatal(err)
	}
	if err := ack.Verify(senderPub, verifyED); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("sender must not authenticate receiver ACK: %v", err)
	}
}

func TestRelayImmutableRenewal(t *testing.T) {
	pub, _ := keyPair(t)
	_, ackKey, err := NewTemporaryKeys()
	if err != nil {
		t.Fatal(err)
	}
	previous := RelayRecord{Version: RecordVersion, TransferID: "t", RecipientID: "r", ObjectHash: [32]byte{1}, ObjectSize: 1, SourcePeerID: "p", AckRecordKey: ackKey, Expiry: 10, SenderPubKey: pub}
	next := previous
	next.Expiry = 20
	next.DKVSBlobRef = "/blob/account/object/manifest"
	if err := next.ValidateRenewal(previous); err != nil {
		t.Fatal(err)
	}
	next.ObjectHash[0] ^= 1
	if err := next.ValidateRenewal(previous); !errors.Is(err, ErrImmutableUpdate) {
		t.Fatalf("expected immutable error, got %v", err)
	}
}

func TestCompactRecordPayloadRoundTripRejectsJSON(t *testing.T) {
	_, ackKey, err := NewTemporaryKeys()
	if err != nil {
		t.Fatal(err)
	}
	senderPub, sender := keyPair(t)
	recipientPub, recipient := keyPair(t)
	relay := RelayRecord{Version: RecordVersion, TransferID: "transfer", RecipientID: "recipient", ObjectHash: [32]byte{1}, ObjectSize: 1, SourcePeerID: "peer", AckRecordKey: ackKey, Expiry: 100, SenderPubKey: senderPub}
	if err := relay.Sign(sender); err != nil {
		t.Fatal(err)
	}
	encodedRelay, err := relay.MarshalBinary()
	if err != nil || len(encodedRelay) == 0 || encodedRelay[0] == '{' {
		t.Fatalf("relay encode err=%v", err)
	}
	decodedRelay, err := UnmarshalRelayRecord(encodedRelay)
	if err != nil || decodedRelay.TransferID != relay.TransferID || decodedRelay.Signature == nil {
		t.Fatalf("relay decode=%+v err=%v", decodedRelay, err)
	}
	if _, err := UnmarshalRelayRecord([]byte(`{"transfer_id":"transfer"}`)); err == nil {
		t.Fatal("JSON relay unexpectedly accepted")
	}
	relayHash, err := relay.Hash()
	if err != nil {
		t.Fatal(err)
	}
	ack := AckRecord{Version: RecordVersion, TransferID: relay.TransferID, RecipientID: relay.RecipientID, RelayRecordHash: relayHash, ConsignmentHash: relay.ObjectHash, Accepted: true, RecipientPubKey: recipientPub}
	if err := ack.Sign(recipient); err != nil {
		t.Fatal(err)
	}
	encodedAck, err := ack.MarshalBinary()
	if err != nil || len(encodedAck) == 0 || encodedAck[0] == '{' {
		t.Fatalf("ack encode err=%v", err)
	}
	decodedAck, err := UnmarshalAckRecord(encodedAck)
	if err != nil || !decodedAck.Accepted || decodedAck.Signature == nil {
		t.Fatalf("ack decode=%+v err=%v", decodedAck, err)
	}
	if _, err := UnmarshalAckRecord([]byte(`{"accepted":true}`)); err == nil {
		t.Fatal("JSON ACK unexpectedly accepted")
	}
}
