package wallet

import (
	"bytes"
	"math"

	"github.com/sat20-labs/rgb11/seals"
	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

const (
	receiveStoreMagic   = "R11W"
	receiveStoreVersion = uint8(1)
	receiveStoreMaxText = 128 * 1024
	receiveStoreMaxBlob = 4 * 1024 * 1024
)

// EncodeReceiveRequest persists the wallet-local receive state using the same
// strict binary primitives as RGB consensus records. It is deliberately not a
// JSON or Gob compatibility format: local records are versioned explicitly.
func EncodeReceiveRequest(request *ReceiveRequest) ([]byte, error) {
	if request == nil || !validReceiveMode(request.Mode) || !validReceiveStatus(request.Status) ||
		request.Version != ReceiveVersion || request.RequestID == "" || request.RecipientID == "" ||
		request.Invoice == "" || request.RelayKey == "" || request.AckKey == "" || request.RelayKey == request.AckKey ||
		request.CreatedAt < 0 || request.Expiry < 0 {
		return nil, ErrInvalidReceive
	}
	seal, err := request.Seal.StrictBytes()
	if err != nil {
		return nil, ErrInvalidReceive
	}
	var buf bytes.Buffer
	e := strict.NewEncoder(&buf)
	for _, write := range []func() error{
		func() error { return e.Raw([]byte(receiveStoreMagic)) },
		func() error { return e.U8(receiveStoreVersion) },
		func() error { return e.U32(request.Version) },
		func() error { return e.String(string(request.Mode), 1, receiveStoreMaxText) },
		func() error { return e.String(request.RequestID, 1, receiveStoreMaxText) },
		func() error { return e.String(request.RecipientID, 1, receiveStoreMaxText) },
		func() error { return e.Bytes(seal, 13, 45) },
		func() error { return e.Bytes(request.WitnessScript, 0, receiveStoreMaxBlob) },
		func() error { return e.String(request.Invoice, 1, receiveStoreMaxText) },
		func() error { return e.String(request.RelayKey, 1, receiveStoreMaxText) },
		func() error { return e.String(request.AckKey, 1, receiveStoreMaxText) },
		func() error { return e.U64(uint64(request.CreatedAt)) },
		func() error { return e.U64(uint64(request.Expiry)) },
		func() error { return e.String(string(request.Status), 1, receiveStoreMaxText) },
		func() error { return e.String(request.TransferID, 0, receiveStoreMaxText) },
		func() error { return e.String(request.ObjectHash, 0, receiveStoreMaxText) },
		func() error { return e.String(request.WitnessTxID, 0, receiveStoreMaxText) },
		func() error { return e.String(request.FailureCode, 0, receiveStoreMaxText) },
	} {
		if err := write(); err != nil {
			return nil, ErrInvalidReceive
		}
	}
	return buf.Bytes(), nil
}

// DecodeReceiveRequest accepts only the explicit strict storage envelope.
// Older JSON records are intentionally rejected instead of being migrated.
func DecodeReceiveRequest(data []byte) (*ReceiveRequest, error) {
	if len(data) == 0 || len(data) > receiveStoreMaxBlob {
		return nil, ErrInvalidReceive
	}
	r := bytes.NewReader(data)
	d := strict.NewDecoder(r)
	magic, err := d.Raw(uint64(len(receiveStoreMagic)))
	if err != nil || string(magic) != receiveStoreMagic {
		return nil, ErrInvalidReceive
	}
	version, err := d.U8()
	if err != nil || version != receiveStoreVersion {
		return nil, ErrInvalidReceive
	}
	request := &ReceiveRequest{}
	if request.Version, err = d.U32(); err != nil || request.Version != ReceiveVersion {
		return nil, ErrInvalidReceive
	}
	mode, err := d.String(1, receiveStoreMaxText)
	if err != nil {
		return nil, ErrInvalidReceive
	}
	request.Mode = ReceiveMode(mode)
	if !validReceiveMode(request.Mode) {
		return nil, ErrInvalidReceive
	}
	if request.RequestID, err = d.String(1, receiveStoreMaxText); err != nil {
		return nil, ErrInvalidReceive
	}
	if request.RecipientID, err = d.String(1, receiveStoreMaxText); err != nil {
		return nil, ErrInvalidReceive
	}
	encodedSeal, err := d.Bytes(13, 45)
	if err != nil {
		return nil, ErrInvalidReceive
	}
	if request.Seal, err = seals.DecodeGraphBlindSeal(encodedSeal); err != nil {
		return nil, ErrInvalidReceive
	}
	if request.WitnessScript, err = d.Bytes(0, receiveStoreMaxBlob); err != nil {
		return nil, ErrInvalidReceive
	}
	for _, target := range []*string{&request.Invoice, &request.RelayKey, &request.AckKey} {
		if *target, err = d.String(1, receiveStoreMaxText); err != nil {
			return nil, ErrInvalidReceive
		}
	}
	createdAt, err := d.U64()
	if err != nil || createdAt > math.MaxInt64 {
		return nil, ErrInvalidReceive
	}
	request.CreatedAt = int64(createdAt)
	expiry, err := d.U64()
	if err != nil || expiry > math.MaxInt64 {
		return nil, ErrInvalidReceive
	}
	request.Expiry = int64(expiry)
	status, err := d.String(1, receiveStoreMaxText)
	if err != nil {
		return nil, ErrInvalidReceive
	}
	request.Status = ReceiveStatus(status)
	if !validReceiveStatus(request.Status) {
		return nil, ErrInvalidReceive
	}
	for _, target := range []*string{&request.TransferID, &request.ObjectHash, &request.WitnessTxID, &request.FailureCode} {
		if *target, err = d.String(0, receiveStoreMaxText); err != nil {
			return nil, ErrInvalidReceive
		}
	}
	if r.Len() != 0 || request.RelayKey == request.AckKey {
		return nil, ErrInvalidReceive
	}
	return request, nil
}

func validReceiveMode(mode ReceiveMode) bool {
	return mode == ReceiveBlind || mode == ReceiveWitness
}

func validReceiveStatus(status ReceiveStatus) bool {
	switch status {
	case ReceivePrepared, ReceiveRelayed, ReceiveAcknowledged, ReceiveAccepted, ReceiveSettled, ReceiveFailed:
		return true
	default:
		return false
	}
}
