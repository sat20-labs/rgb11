package strict_encoding

import (
	"encoding/binary"
	"errors"
	"io"
)

// Upstream-Repository: rgb-protocol/rgb-strict-encoding
// Upstream-Version: 1.0.2
// Upstream-Commit: 7698a5e96a2a27d5bfa4cd3560da0e8af8e4a18a
// Upstream-File: rust/src/traits.rs
// Upstream-File-SHA256: ddec82128f25d29db095ed325eb4f4906cb3e6c25894efca016c125a5f8a4235
// Translation-Revision: 1

type Decoder struct {
	r io.Reader
}

func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: r} }

func (d *Decoder) Raw(size uint64) ([]byte, error) {
	if size > uint64(^uint(0)>>1) {
		return nil, ErrOutOfBounds
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(d.r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrUnexpectedEOF
		}
		return nil, err
	}
	return buf, nil
}

func (d *Decoder) U8() (uint8, error) {
	buf, err := d.Raw(1)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (d *Decoder) U16() (uint16, error) {
	buf, err := d.Raw(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func (d *Decoder) U24() (uint32, error) {
	buf, err := d.Raw(3)
	if err != nil {
		return 0, err
	}
	return uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16, nil
}

func (d *Decoder) U32() (uint32, error) {
	buf, err := d.Raw(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (d *Decoder) U64() (uint64, error) {
	buf, err := d.Raw(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

func (d *Decoder) Bool() (bool, error) {
	v, err := d.U8()
	if err != nil {
		return false, err
	}
	switch v {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, ErrInvalidBool
	}
}

func (d *Decoder) Length(max uint64) (uint64, error) {
	var value uint64
	var err error
	switch {
	case max <= 0xFF:
		var v uint8
		v, err = d.U8()
		value = uint64(v)
	case max <= 0xFFFF:
		var v uint16
		v, err = d.U16()
		value = uint64(v)
	case max <= 0xFFFFFF:
		var v uint32
		v, err = d.U24()
		value = uint64(v)
	case max <= 0xFFFFFFFF:
		var v uint32
		v, err = d.U32()
		value = uint64(v)
	default:
		value, err = d.U64()
	}
	if err != nil {
		return 0, err
	}
	if value > max {
		return 0, ErrOutOfBounds
	}
	return value, nil
}

func (d *Decoder) Bytes(min, max uint64) ([]byte, error) {
	length, err := d.Length(max)
	if err != nil {
		return nil, err
	}
	if length < min {
		return nil, ErrOutOfBounds
	}
	return d.Raw(length)
}

func (d *Decoder) String(min, max uint64) (string, error) {
	b, err := d.Bytes(min, max)
	return string(b), err
}

func (d *Decoder) Option(decode func(*Decoder) error) (bool, error) {
	tag, err := d.U8()
	if err != nil {
		return false, err
	}
	switch tag {
	case 0:
		return false, nil
	case 1:
		if decode == nil {
			return false, ErrInvalidOptionTag
		}
		return true, decode(d)
	default:
		return false, ErrInvalidOptionTag
	}
}
