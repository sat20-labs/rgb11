// Package strict_encoding implements the deterministic binary primitives used
// by RGB strict-encoding 1.0.2. Composite RGB types explicitly call these
// primitives in the same field order as the frozen Rust source.
package strict_encoding

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Upstream-Repository: rgb-protocol/rgb-strict-encoding
// Upstream-Version: 1.0.2
// Upstream-Commit: 7698a5e96a2a27d5bfa4cd3560da0e8af8e4a18a
// Upstream-File: rust/src/traits.rs
// Upstream-File-SHA256: ddec82128f25d29db095ed325eb4f4906cb3e6c25894efca016c125a5f8a4235
// Translation-Revision: 1

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

func (e *Encoder) Raw(data []byte) error {
	_, err := e.w.Write(data)
	return err
}

func (e *Encoder) U8(v uint8) error { return e.Raw([]byte{v}) }

func (e *Encoder) U16(v uint16) error {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return e.Raw(buf[:])
}

func (e *Encoder) U24(v uint32) error {
	if v > 0xFFFFFF {
		return ErrOutOfBounds
	}
	return e.Raw([]byte{byte(v), byte(v >> 8), byte(v >> 16)})
}

func (e *Encoder) U32(v uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	return e.Raw(buf[:])
}

func (e *Encoder) U64(v uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	return e.Raw(buf[:])
}

func (e *Encoder) Bool(v bool) error {
	if v {
		return e.U8(1)
	}
	return e.U8(0)
}

// Length writes the collection length using the width selected by the
// collection's declared maximum, matching WriteRaw::write_raw_len.
func (e *Encoder) Length(length, max uint64) error {
	if length > max {
		return ErrOutOfBounds
	}
	switch {
	case max <= 0xFF:
		return e.U8(uint8(length))
	case max <= 0xFFFF:
		return e.U16(uint16(length))
	case max <= 0xFFFFFF:
		return e.U24(uint32(length))
	case max <= 0xFFFFFFFF:
		return e.U32(uint32(length))
	default:
		return e.U64(length)
	}
}

func (e *Encoder) Bytes(data []byte, min, max uint64) error {
	if uint64(len(data)) < min || uint64(len(data)) > max {
		return ErrOutOfBounds
	}
	if err := e.Length(uint64(len(data)), max); err != nil {
		return err
	}
	return e.Raw(data)
}

func (e *Encoder) String(value string, min, max uint64) error {
	return e.Bytes([]byte(value), min, max)
}

func (e *Encoder) Option(present bool, encode func(*Encoder) error) error {
	if !present {
		return e.U8(0)
	}
	if encode == nil {
		return fmt.Errorf("strict encoding option: nil encoder")
	}
	if err := e.U8(1); err != nil {
		return err
	}
	return encode(e)
}
