package strict_types

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
)

type ValueKind uint8

const (
	ValueUnit ValueKind = iota
	ValueNumber
	ValueBytes
	ValueString
	ValueEnum
	ValueUnion
	ValueTuple
	ValueStruct
	ValueList
	ValueSet
	ValueMap
)

type Value struct {
	Kind      ValueKind
	Encoded   []byte
	Primitive uint8
	Raw       []byte
	Unsigned  *uint64
	Signed    *int64
	Text      string
	Name      string
	Tag       uint8
	Inner     *Value
	Fields    []Field
	Items     []Value
	Entries   []Entry
}

type Field struct {
	Name  string
	Value Value
}

type Entry struct {
	Key   Value
	Value Value
}

// Clone returns a deep copy suitable for constructing a new strict value from
// a decoded template without mutating the original consignment.
func (v Value) Clone() Value {
	clone := v
	clone.Encoded = append([]byte(nil), v.Encoded...)
	clone.Raw = append([]byte(nil), v.Raw...)
	if v.Unsigned != nil {
		value := *v.Unsigned
		clone.Unsigned = &value
	}
	if v.Signed != nil {
		value := *v.Signed
		clone.Signed = &value
	}
	if v.Inner != nil {
		value := v.Inner.Clone()
		clone.Inner = &value
	}
	clone.Fields = make([]Field, len(v.Fields))
	for index, field := range v.Fields {
		clone.Fields[index] = Field{Name: field.Name, Value: field.Value.Clone()}
	}
	clone.Items = make([]Value, len(v.Items))
	for index := range v.Items {
		clone.Items[index] = v.Items[index].Clone()
	}
	clone.Entries = make([]Entry, len(v.Entries))
	for index, entry := range v.Entries {
		clone.Entries[index] = Entry{Key: entry.Key.Clone(), Value: entry.Value.Clone()}
	}
	return clone
}

func (v Value) Field(name string) (Value, bool) {
	if v.Kind != ValueStruct {
		return Value{}, false
	}
	for _, field := range v.Fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return Value{}, false
}

func (v Value) Uint64() (uint64, bool) {
	if v.Kind != ValueNumber || v.Unsigned == nil {
		return 0, false
	}
	return *v.Unsigned, true
}

func (v Value) Unwrap() Value {
	for v.Kind == ValueTuple && len(v.Items) == 1 {
		v = v.Items[0]
	}
	return v
}

func (v Value) Bytes() ([]byte, bool) {
	v = v.Unwrap()
	if v.Kind != ValueBytes {
		return nil, false
	}
	return append([]byte(nil), v.Raw...), true
}

func (v Value) Bool() (bool, bool) {
	if v.Kind != ValueEnum || (v.Tag != 0 && v.Tag != 1) {
		return false, false
	}
	return v.Tag == 1, true
}

func (v Value) String() string {
	switch v.Kind {
	case ValueString, ValueEnum:
		return v.Text
	case ValueBytes:
		return fmt.Sprintf("%x", v.Raw)
	default:
		return ""
	}
}

// TextValue reconstructs strict string wrapper types. Strict Types represents
// identifiers as nested tuples containing a leading character and a confined
// list of remaining characters, so a direct Value.String call is not enough.
func (v Value) TextValue() (string, bool) {
	switch v.Kind {
	case ValueString:
		return v.Text, true
	case ValueTuple, ValueList, ValueSet:
		var builder strings.Builder
		for _, item := range v.Items {
			text, ok := item.TextValue()
			if !ok {
				return "", false
			}
			builder.WriteString(text)
		}
		return builder.String(), true
	default:
		return "", false
	}
}

// compareValue implements the semantic ordering used by Rust-derived strict
// collection types. Canonical set/map order is based on the value's Ord
// implementation, not on lexicographic comparison of its little-endian wire
// encoding.
func compareValue(left, right Value) (int, bool) {
	left = left.Unwrap()
	right = right.Unwrap()
	if left.Kind != right.Kind {
		return 0, false
	}
	switch left.Kind {
	case ValueUnit:
		return 0, true
	case ValueNumber:
		return compareNumbers(left, right)
	case ValueBytes:
		return bytes.Compare(left.Raw, right.Raw), true
	case ValueString:
		return strings.Compare(left.Text, right.Text), true
	case ValueEnum:
		return compareUint(uint64(left.Tag), uint64(right.Tag)), true
	case ValueUnion:
		if order := compareUint(uint64(left.Tag), uint64(right.Tag)); order != 0 {
			return order, true
		}
		if left.Inner == nil || right.Inner == nil {
			return 0, left.Inner == nil && right.Inner == nil
		}
		return compareValue(*left.Inner, *right.Inner)
	case ValueTuple, ValueList, ValueSet:
		return compareValues(left.Items, right.Items)
	case ValueStruct:
		if len(left.Fields) != len(right.Fields) {
			return 0, false
		}
		// aluvm::library::Lib implements Ord by its tagged LibId, rather than
		// by the wire-order of the Lib struct fields. RGB consignments store
		// scripts in a strict Set<Lib>; using structural ordering here rejects
		// canonical multi-script contracts such as the official IFA fixture.
		if leftID, leftOK := aluvmLibID(left); leftOK {
			rightID, rightOK := aluvmLibID(right)
			if !rightOK {
				return 0, false
			}
			return bytes.Compare(leftID[:], rightID[:]), true
		}
		// strict_encoding::Variant deliberately orders by tag (and treats a
		// repeated name as equal), even though its wire struct stores name first.
		if len(left.Fields) == 2 && left.Fields[0].Name == "name" && left.Fields[1].Name == "tag" &&
			right.Fields[0].Name == "name" && right.Fields[1].Name == "tag" {
			leftTag, leftOK := left.Fields[1].Value.Uint64()
			rightTag, rightOK := right.Fields[1].Value.Uint64()
			if !leftOK || !rightOK {
				return 0, false
			}
			return compareUint(leftTag, rightTag), true
		}
		for index := range left.Fields {
			if left.Fields[index].Name != right.Fields[index].Name {
				return 0, false
			}
			order, ok := compareValue(left.Fields[index].Value, right.Fields[index].Value)
			if !ok || order != 0 {
				return order, ok
			}
		}
		return 0, true
	case ValueMap:
		if len(left.Entries) != len(right.Entries) {
			return compareUint(uint64(len(left.Entries)), uint64(len(right.Entries))), true
		}
		for index := range left.Entries {
			order, ok := compareValue(left.Entries[index].Key, right.Entries[index].Key)
			if !ok || order != 0 {
				return order, ok
			}
			order, ok = compareValue(left.Entries[index].Value, right.Entries[index].Value)
			if !ok || order != 0 {
				return order, ok
			}
		}
		return 0, true
	default:
		return 0, false
	}
}

func aluvmLibID(value Value) ([32]byte, bool) {
	var zero [32]byte
	if len(value.Fields) != 4 || value.Fields[0].Name != "isae" || value.Fields[1].Name != "code" ||
		value.Fields[2].Name != "data" || value.Fields[3].Name != "libs" {
		return zero, false
	}
	isaeValue := value.Fields[0].Value.Unwrap()
	if isaeValue.Kind != ValueSet || len(isaeValue.Items) > 255 {
		return zero, false
	}
	isaeNames := make([]string, 0, len(isaeValue.Items))
	for _, item := range isaeValue.Items {
		name, ok := item.TextValue()
		if !ok {
			return zero, false
		}
		isaeNames = append(isaeNames, name)
	}
	isae := []byte(strings.Join(isaeNames, " "))
	code, codeOK := value.Fields[1].Value.Bytes()
	data, dataOK := value.Fields[2].Value.Bytes()
	libsValue := value.Fields[3].Value.Unwrap()
	if !codeOK || !dataOK || len(isae) > 255 || len(code) > 65535 || len(data) > 65535 ||
		libsValue.Kind != ValueSet || len(libsValue.Items) > 255 {
		return zero, false
	}
	libs := make([][32]byte, 0, len(libsValue.Items))
	for _, item := range libsValue.Items {
		raw, ok := item.Bytes()
		if !ok || len(raw) != 32 {
			return zero, false
		}
		var lib [32]byte
		copy(lib[:], raw)
		libs = append(libs, lib)
	}

	const tag = "urn:ubideco:aluvm:lib:v01#230304"
	tagHash := sha256.Sum256([]byte(tag))
	hasher := sha256.New()
	hasher.Write(tagHash[:])
	hasher.Write(tagHash[:])
	hasher.Write([]byte{byte(len(isae))})
	hasher.Write(isae)
	hasher.Write([]byte{byte(len(code)), byte(len(code) >> 8)})
	hasher.Write(code)
	hasher.Write([]byte{byte(len(data)), byte(len(data) >> 8)})
	hasher.Write(data)
	hasher.Write([]byte{byte(len(libs))})
	for _, lib := range libs {
		hasher.Write(lib[:])
	}
	copy(zero[:], hasher.Sum(nil))
	return zero, true
}

func compareValues(left, right []Value) (int, bool) {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		order, ok := compareValue(left[index], right[index])
		if !ok || order != 0 {
			return order, ok
		}
	}
	return compareUint(uint64(len(left)), uint64(len(right))), true
}

func compareNumbers(left, right Value) (int, bool) {
	if left.Unsigned != nil && right.Unsigned != nil {
		return compareUint(*left.Unsigned, *right.Unsigned), true
	}
	if left.Signed != nil && right.Signed != nil {
		switch {
		case *left.Signed < *right.Signed:
			return -1, true
		case *left.Signed > *right.Signed:
			return 1, true
		default:
			return 0, true
		}
	}
	if left.Primitive != right.Primitive || len(left.Raw) != len(right.Raw) {
		return 0, false
	}
	leftInt := littleEndianBigInt(left.Raw, left.Primitive&0xc0 == 0x40)
	rightInt := littleEndianBigInt(right.Raw, right.Primitive&0xc0 == 0x40)
	return leftInt.Cmp(rightInt), true
}

func littleEndianBigInt(raw []byte, signed bool) *big.Int {
	bigEndian := make([]byte, len(raw))
	for index := range raw {
		bigEndian[len(raw)-1-index] = raw[index]
	}
	value := new(big.Int).SetBytes(bigEndian)
	if signed && len(raw) > 0 && raw[len(raw)-1]&0x80 != 0 {
		modulus := new(big.Int).Lsh(big.NewInt(1), uint(len(raw)*8))
		value.Sub(value, modulus)
	}
	return value
}

func compareUint(left, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
