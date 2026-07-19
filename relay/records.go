// Package relay defines SAT20's signed DKVS /tmp envelopes for RGB11 object
// delivery. Consignment bytes remain in the local object store unless an
// explicit DKVS blob backup exists.
package relay

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

const RecordVersion uint32 = 1

type Durability string

const (
	LocalOnly     Durability = "LOCAL_ONLY"
	RelayedTemp   Durability = "RELAYED_TEMP"
	DKVSBackedUp  Durability = "DKVS_BACKED_UP"
	BackupExpired Durability = "BACKUP_EXPIRED"
)

var (
	ErrInvalidRecord    = errors.New("invalid RGB11 relay record")
	ErrInvalidSignature = errors.New("invalid RGB11 relay signature")
	ErrExpired          = errors.New("RGB11 relay record expired")
	ErrImmutableUpdate  = errors.New("RGB11 relay immutable field changed")
	ErrInvalidKey       = errors.New("invalid RGB11 DKVS temporary key")
)

type Signer interface {
	Sign(message []byte) ([]byte, error)
}

type VerifySignature func(pubKey, message, signature []byte) bool

type RelayRecord struct {
	Version       uint32   `json:"version"`
	TransferID    string   `json:"transfer_id"`
	RecipientID   string   `json:"recipient_id"`
	ObjectHash    [32]byte `json:"object_hash"`
	ObjectSize    uint64   `json:"object_size"`
	LocalObjectID string   `json:"local_object_id,omitempty"`
	DKVSBlobRef   string   `json:"dkvs_blob_ref,omitempty"`
	SourcePeerID  string   `json:"source_peer_id"`
	WitnessTxID   string   `json:"witness_txid,omitempty"`
	AckRecordKey  string   `json:"ack_record_key"`
	Expiry        int64    `json:"expiry"`
	SenderPubKey  []byte   `json:"sender_pubkey"`
	Signature     []byte   `json:"signature"`
}

type AckRecord struct {
	Version         uint32   `json:"version"`
	TransferID      string   `json:"transfer_id"`
	RecipientID     string   `json:"recipient_id"`
	RelayRecordHash [32]byte `json:"relay_record_hash"`
	ConsignmentHash [32]byte `json:"consignment_hash"`
	Accepted        bool     `json:"accepted"`
	ReasonCode      string   `json:"reason_code,omitempty"`
	RecipientPubKey []byte   `json:"recipient_pubkey"`
	Signature       []byte   `json:"signature"`
}

func NewTemporaryKeys() (relayKey, ackKey string, err error) {
	ids := make([]byte, 64)
	if _, err := rand.Read(ids); err != nil {
		return "", "", err
	}
	return "/tmp/" + hex.EncodeToString(ids[:32]), "/tmp/" + hex.EncodeToString(ids[32:]), nil
}

func ValidateTemporaryKey(key string) error {
	if !strings.HasPrefix(key, "/tmp/") {
		return ErrInvalidKey
	}
	id := strings.TrimPrefix(key, "/tmp/")
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 {
		return ErrInvalidKey
	}
	return nil
}

func writeString(e *strict.Encoder, value string, max uint64) error {
	return e.String(value, 0, max)
}

func (r RelayRecord) validateFields() error {
	if r.Version != RecordVersion || r.TransferID == "" || r.RecipientID == "" || r.ObjectSize == 0 ||
		r.Expiry <= 0 || len(r.SenderPubKey) == 0 || len(r.SenderPubKey) > 128 ||
		(r.SourcePeerID == "" && r.DKVSBlobRef == "") {
		return ErrInvalidRecord
	}
	if err := ValidateTemporaryKey(r.AckRecordKey); err != nil {
		return err
	}
	return nil
}

func (r RelayRecord) SigningBytes() ([]byte, error) {
	if err := r.validateFields(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	e := strict.NewEncoder(&buf)
	for _, encode := range []func() error{
		func() error { return e.U32(r.Version) },
		func() error { return writeString(e, r.TransferID, 128) },
		func() error { return writeString(e, r.RecipientID, 256) },
		func() error { return e.Raw(r.ObjectHash[:]) },
		func() error { return e.U64(r.ObjectSize) },
		func() error { return writeString(e, r.LocalObjectID, 128) },
		func() error { return writeString(e, r.DKVSBlobRef, 512) },
		func() error { return writeString(e, r.SourcePeerID, 128) },
		func() error { return writeString(e, r.WitnessTxID, 64) },
		func() error { return writeString(e, r.AckRecordKey, 128) },
		func() error { return e.U64(uint64(r.Expiry)) },
		func() error { return e.Bytes(r.SenderPubKey, 1, 128) },
	} {
		if err := encode(); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (r *RelayRecord) Sign(signer Signer) error {
	if signer == nil {
		return ErrInvalidSignature
	}
	message, err := r.SigningBytes()
	if err != nil {
		return err
	}
	r.Signature, err = signer.Sign(message)
	if err != nil {
		return err
	}
	if len(r.Signature) == 0 || len(r.Signature) > 256 {
		return ErrInvalidSignature
	}
	return nil
}

func (r RelayRecord) Verify(expectedSenderPubKey []byte, now int64, verify VerifySignature) error {
	if err := r.validateFields(); err != nil {
		return err
	}
	if !bytes.Equal(r.SenderPubKey, expectedSenderPubKey) || verify == nil || len(r.Signature) == 0 {
		return ErrInvalidSignature
	}
	if now >= r.Expiry {
		return ErrExpired
	}
	message, err := r.SigningBytes()
	if err != nil {
		return err
	}
	if !verify(r.SenderPubKey, message, r.Signature) {
		return ErrInvalidSignature
	}
	return nil
}

func (r RelayRecord) Hash() ([32]byte, error) {
	message, err := r.SigningBytes()
	if err != nil {
		return [32]byte{}, err
	}
	if len(r.Signature) == 0 || len(r.Signature) > 256 {
		return [32]byte{}, ErrInvalidSignature
	}
	var buf bytes.Buffer
	buf.Write(message)
	if err := strict.NewEncoder(&buf).Bytes(r.Signature, 1, 256); err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

// ValidateRenewal permits locator, witness and expiry updates while preserving
// the transfer result and object identity.
func (r RelayRecord) ValidateRenewal(previous RelayRecord) error {
	if r.TransferID != previous.TransferID || r.RecipientID != previous.RecipientID ||
		r.ObjectHash != previous.ObjectHash || r.ObjectSize != previous.ObjectSize ||
		r.AckRecordKey != previous.AckRecordKey || !bytes.Equal(r.SenderPubKey, previous.SenderPubKey) {
		return ErrImmutableUpdate
	}
	if r.Expiry <= previous.Expiry {
		return fmt.Errorf("%w: expiry did not increase", ErrImmutableUpdate)
	}
	return nil
}

func (a AckRecord) validateFields() error {
	if a.Version != RecordVersion || a.TransferID == "" || a.RecipientID == "" ||
		len(a.RecipientPubKey) == 0 || len(a.RecipientPubKey) > 128 ||
		(!a.Accepted && a.ReasonCode == "") {
		return ErrInvalidRecord
	}
	return nil
}

func (a AckRecord) SigningBytes() ([]byte, error) {
	if err := a.validateFields(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	e := strict.NewEncoder(&buf)
	for _, encode := range []func() error{
		func() error { return e.U32(a.Version) },
		func() error { return writeString(e, a.TransferID, 128) },
		func() error { return writeString(e, a.RecipientID, 256) },
		func() error { return e.Raw(a.RelayRecordHash[:]) },
		func() error { return e.Raw(a.ConsignmentHash[:]) },
		func() error { return e.Bool(a.Accepted) },
		func() error { return writeString(e, a.ReasonCode, 128) },
		func() error { return e.Bytes(a.RecipientPubKey, 1, 128) },
	} {
		if err := encode(); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (a *AckRecord) Sign(signer Signer) error {
	if signer == nil {
		return ErrInvalidSignature
	}
	message, err := a.SigningBytes()
	if err != nil {
		return err
	}
	a.Signature, err = signer.Sign(message)
	if err != nil {
		return err
	}
	if len(a.Signature) == 0 || len(a.Signature) > 256 {
		return ErrInvalidSignature
	}
	return nil
}

func (a AckRecord) Verify(expectedRecipientPubKey []byte, verify VerifySignature) error {
	if err := a.validateFields(); err != nil {
		return err
	}
	if !bytes.Equal(a.RecipientPubKey, expectedRecipientPubKey) || verify == nil || len(a.Signature) == 0 {
		return ErrInvalidSignature
	}
	message, err := a.SigningBytes()
	if err != nil {
		return err
	}
	if !verify(a.RecipientPubKey, message, a.Signature) {
		return ErrInvalidSignature
	}
	return nil
}

func (a AckRecord) ValidateImmutable(previous AckRecord) error {
	if a.TransferID != previous.TransferID || a.RecipientID != previous.RecipientID ||
		a.RelayRecordHash != previous.RelayRecordHash || a.ConsignmentHash != previous.ConsignmentHash ||
		a.Accepted != previous.Accepted || a.ReasonCode != previous.ReasonCode ||
		!bytes.Equal(a.RecipientPubKey, previous.RecipientPubKey) {
		return ErrImmutableUpdate
	}
	return nil
}
