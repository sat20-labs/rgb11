// Package psbt implements the RGB 0.11.1 proprietary PSBT key namespace used
// by rgb-psbt-utils. It intentionally operates on raw key/value maps so the
// engine can adapt to either btcd PSBT implementation used by SAT20.
package psbt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Upstream-Repository: rgb-protocol/rgb-api
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 8d448f46c866d44ca0495ad0e924e57d9fd294dd
// Upstream-File: psbt/src/lib.rs
// Translation-Revision: 1

var (
	MPCPrefix    = []byte("MPC")
	OpretPrefix  = []byte("OPRET")
	TapretPrefix = []byte("TAPRET")
	RGBPrefix    = []byte("RGB")
)

const (
	OutMPCMessage      uint8 = 0x00
	OutMPCEntropy      uint8 = 0x01
	OutMPCMinTreeDepth uint8 = 0x04
	OutMPCCommitment   uint8 = 0x10
	OutMPCProof        uint8 = 0x11

	OutOpretHost       uint8 = 0x00
	OutOpretCommitment uint8 = 0x01

	OutTapretHost       uint8 = 0x00
	OutTapretCommitment uint8 = 0x01
	OutTapretProof      uint8 = 0x02

	GlobalRGBTransition    uint8 = 0x01
	GlobalRGBCloseMethod   uint8 = 0x02
	GlobalRGBTapHostChange uint8 = 0x03
	GlobalRGBConsumedBy    uint8 = 0x04
	ProprietaryKeyType     uint8 = 0xFC
)

var ErrInvalidProprietaryKey = errors.New("invalid RGB11 proprietary PSBT key")

type ProprietaryKey struct {
	Prefix  []byte
	Subtype uint8
	Key     []byte
}

func NewKey(prefix []byte, subtype uint8, key []byte) ProprietaryKey {
	return ProprietaryKey{Prefix: append([]byte(nil), prefix...), Subtype: subtype, Key: append([]byte(nil), key...)}
}

func MPCMessage(protocolID [32]byte) ProprietaryKey {
	return NewKey(MPCPrefix, OutMPCMessage, protocolID[:])
}
func MPCEntropy() ProprietaryKey       { return NewKey(MPCPrefix, OutMPCEntropy, nil) }
func MPCMinTreeDepth() ProprietaryKey  { return NewKey(MPCPrefix, OutMPCMinTreeDepth, nil) }
func MPCCommitment() ProprietaryKey    { return NewKey(MPCPrefix, OutMPCCommitment, nil) }
func MPCProof() ProprietaryKey         { return NewKey(MPCPrefix, OutMPCProof, nil) }
func OpretHost() ProprietaryKey        { return NewKey(OpretPrefix, OutOpretHost, nil) }
func OpretCommitment() ProprietaryKey  { return NewKey(OpretPrefix, OutOpretCommitment, nil) }
func TapretHost() ProprietaryKey       { return NewKey(TapretPrefix, OutTapretHost, nil) }
func TapretCommitment() ProprietaryKey { return NewKey(TapretPrefix, OutTapretCommitment, nil) }
func TapretProof() ProprietaryKey      { return NewKey(TapretPrefix, OutTapretProof, nil) }
func RGBTransition(operationID [32]byte) ProprietaryKey {
	return NewKey(RGBPrefix, GlobalRGBTransition, operationID[:])
}
func RGBCloseMethod() ProprietaryKey   { return NewKey(RGBPrefix, GlobalRGBCloseMethod, nil) }
func RGBTapHostChange() ProprietaryKey { return NewKey(RGBPrefix, GlobalRGBTapHostChange, nil) }
func RGBConsumedBy(contractID [32]byte) ProprietaryKey {
	return NewKey(RGBPrefix, GlobalRGBConsumedBy, contractID[:])
}

// KeyData serializes bitcoin::psbt::raw::ProprietaryKey: CompactSize prefix
// length, prefix, subtype and application key data.
func (k ProprietaryKey) KeyData() ([]byte, error) {
	if len(k.Prefix) == 0 || len(k.Prefix) > 1024 || len(k.Key) > 1024 {
		return nil, ErrInvalidProprietaryKey
	}
	var buf bytes.Buffer
	if err := writeCompactSize(&buf, uint64(len(k.Prefix))); err != nil {
		return nil, err
	}
	buf.Write(k.Prefix)
	buf.WriteByte(k.Subtype)
	buf.Write(k.Key)
	return buf.Bytes(), nil
}

// RawKey returns the PSBT in-memory key bytes used by btcd and rust-bitcoin:
// the proprietary type byte followed by the proprietary key data. The PSBT
// serializer itself adds the outer CompactSize key length.
func (k ProprietaryKey) RawKey() ([]byte, error) {
	data, err := k.KeyData()
	if err != nil {
		return nil, err
	}
	return append([]byte{ProprietaryKeyType}, data...), nil
}

// SerializeKey serializes the complete BIP174 raw key including key length and
// the 0xFC proprietary type byte.
func (k ProprietaryKey) SerializeKey() ([]byte, error) {
	data, err := k.KeyData()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCompactSize(&buf, uint64(1+len(data))); err != nil {
		return nil, err
	}
	buf.WriteByte(ProprietaryKeyType)
	buf.Write(data)
	return buf.Bytes(), nil
}

func ParseKeyData(data []byte) (ProprietaryKey, error) {
	r := bytes.NewReader(data)
	prefixLength, err := readCompactSize(r)
	if err != nil || prefixLength == 0 || prefixLength > 1024 || prefixLength+1 > uint64(r.Len()) {
		return ProprietaryKey{}, ErrInvalidProprietaryKey
	}
	prefix := make([]byte, prefixLength)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return ProprietaryKey{}, ErrInvalidProprietaryKey
	}
	subtype, err := r.ReadByte()
	if err != nil {
		return ProprietaryKey{}, ErrInvalidProprietaryKey
	}
	key := make([]byte, r.Len())
	_, _ = io.ReadFull(r, key)
	return NewKey(prefix, subtype, key), nil
}

func writeCompactSize(w io.Writer, value uint64) error {
	var buf [9]byte
	switch {
	case value < 0xFD:
		buf[0] = byte(value)
		_, err := w.Write(buf[:1])
		return err
	case value <= 0xFFFF:
		buf[0] = 0xFD
		binary.LittleEndian.PutUint16(buf[1:3], uint16(value))
		_, err := w.Write(buf[:3])
		return err
	case value <= 0xFFFFFFFF:
		buf[0] = 0xFE
		binary.LittleEndian.PutUint32(buf[1:5], uint32(value))
		_, err := w.Write(buf[:5])
		return err
	default:
		buf[0] = 0xFF
		binary.LittleEndian.PutUint64(buf[1:9], value)
		_, err := w.Write(buf[:9])
		return err
	}
}

func readCompactSize(r io.Reader) (uint64, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, err
	}
	switch first[0] {
	case 0xFD:
		var buf [2]byte
		_, err := io.ReadFull(r, buf[:])
		value := uint64(binary.LittleEndian.Uint16(buf[:]))
		if err != nil || value < 0xFD {
			return 0, ErrInvalidProprietaryKey
		}
		return value, nil
	case 0xFE:
		var buf [4]byte
		_, err := io.ReadFull(r, buf[:])
		value := uint64(binary.LittleEndian.Uint32(buf[:]))
		if err != nil || value <= 0xFFFF {
			return 0, ErrInvalidProprietaryKey
		}
		return value, nil
	case 0xFF:
		var buf [8]byte
		_, err := io.ReadFull(r, buf[:])
		value := binary.LittleEndian.Uint64(buf[:])
		if err != nil || value <= 0xFFFFFFFF {
			return 0, ErrInvalidProprietaryKey
		}
		return value, nil
	default:
		return uint64(first[0]), nil
	}
}

type Terminal struct {
	Keychain uint8
	Index    uint32
}

func ParseTerminal(value string) (Terminal, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "&") {
		return Terminal{}, fmt.Errorf("%w: terminal %q", ErrInvalidProprietaryKey, value)
	}
	keychain, err := strconv.ParseUint(strings.TrimPrefix(parts[0], "&"), 10, 8)
	if err != nil {
		return Terminal{}, err
	}
	index, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return Terminal{}, err
	}
	return Terminal{Keychain: uint8(keychain), Index: uint32(index)}, nil
}

func (t Terminal) String() string {
	return "&" + strconv.FormatUint(uint64(t.Keychain), 10) + "/" + strconv.FormatUint(uint64(t.Index), 10)
}
