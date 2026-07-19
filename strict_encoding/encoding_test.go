package strict_encoding

import (
	"bytes"
	"errors"
	"testing"
)

func TestLengthWidthFollowsMaximum(t *testing.T) {
	tests := []struct {
		max  uint64
		want []byte
	}{
		{0xFF, []byte{0x2A}},
		{0xFFFF, []byte{0x2A, 0x00}},
		{0xFFFFFF, []byte{0x2A, 0x00, 0x00}},
		{0xFFFFFFFF, []byte{0x2A, 0x00, 0x00, 0x00}},
		{0x100000000, []byte{0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, test := range tests {
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Length(42, test.max); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), test.want) {
			t.Fatalf("max=%x got=%x want=%x", test.max, buf.Bytes(), test.want)
		}
		got, err := NewDecoder(bytes.NewReader(buf.Bytes())).Length(test.max)
		if err != nil || got != 42 {
			t.Fatalf("max=%x decoded=%d err=%v", test.max, got, err)
		}
	}
}

func TestConfinement(t *testing.T) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Bytes([]byte{1, 2, 3}, 0, 2); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestInvalidOptionTag(t *testing.T) {
	_, err := NewDecoder(bytes.NewReader([]byte{2})).Option(func(*Decoder) error { return nil })
	if !errors.Is(err, ErrInvalidOptionTag) {
		t.Fatalf("expected ErrInvalidOptionTag, got %v", err)
	}
}
