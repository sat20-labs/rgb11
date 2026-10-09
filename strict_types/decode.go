package strict_types

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

// Upstream-Repository: rgb-protocol/rgb-strict-types
// Upstream-Version: 1.0.4
// Upstream-Commit: 9eb3b484f23b01c70f2a940bda07a207377b3864
// Upstream-File: src/value/decode.rs
// Upstream-File-SHA256: f254e3dd82c18ca116d2bbae6f3b55bfbf11130d7820d7584b2730e619046464
// Translation-Revision: 1

var (
	ErrMalformedType  = errors.New("malformed strict type definition")
	ErrUnknownVariant = errors.New("unknown strict type variant")
	ErrNonCanonical   = errors.New("non-canonical strict collection")
)

type decoder struct {
	registry *Registry
	reader   *bytes.Reader
	strict   *strict.Decoder
}

type typeRef struct {
	Named  *string
	Extern *struct {
		LibID string `json:"libId"`
		SemID string `json:"semId"`
	}
	Inline *parsedType
}

type sizing struct {
	Min uint64 `json:"min"`
	Max uint64 `json:"max"`
}

func (r *Registry) Decode(libraryName, typeName string, data []byte) (Value, error) {
	lib, err := r.library(libraryName)
	if err != nil {
		return Value{}, err
	}
	typeDef, err := lib.named(typeName)
	if err != nil {
		return Value{}, err
	}
	reader := bytes.NewReader(data)
	d := &decoder{registry: r, reader: reader, strict: strict.NewDecoder(reader)}
	value, err := d.decodeType(lib, typeDef, libraryName+"."+typeName)
	if err != nil {
		return Value{}, err
	}
	if reader.Len() != 0 {
		return Value{}, strict.ErrTrailingData
	}
	return value, nil
}

func (d *decoder) decodeType(lib *typeLibrary, typeDef *parsedType, path string) (Value, error) {
	start := d.reader.Len()
	value, err := d.decodeTypeBody(lib, typeDef, path)
	if err != nil {
		return Value{}, err
	}
	consumed := start - d.reader.Len()
	value.Encoded = make([]byte, consumed)
	if consumed > 0 {
		position := int64(d.reader.Size()) - int64(start)
		if _, err := d.reader.ReadAt(value.Encoded, position); err != nil {
			return Value{}, err
		}
	}
	return value, nil
}

func (d *decoder) decodeTypeBody(lib *typeLibrary, typeDef *parsedType, path string) (Value, error) {
	if typeDef == nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	switch typeDef.kind {
	case "Primitive":
		return d.decodePrimitive(typeDef.primitive, path)
	case "UnicodeChar", "Unicode":
		return d.decodeUnicode(path)
	case "Enum":
		return d.decodeEnum(typeDef.variants, path)
	case "Union":
		return d.decodeUnion(lib, typeDef.variants, path)
	case "Tuple":
		return d.decodeTuple(lib, typeDef.refs, path)
	case "Struct":
		return d.decodeStruct(lib, typeDef.fields, path)
	case "Array":
		return d.decodeArray(lib, typeDef.refs[0], typeDef.length, path)
	case "List", "Set":
		return d.decodeList(lib, typeDef.kind, typeDef.refs[0], typeDef.limits, path)
	case "Map":
		return d.decodeMap(lib, typeDef.refs, typeDef.limits, path)
	default:
		return Value{}, fmt.Errorf("%s: %w %q", path, ErrMalformedType, typeDef.kind)
	}
}

func (d *decoder) decodeRef(lib *typeLibrary, ref typeRef, path string) (Value, error) {
	switch {
	case ref.Named != nil:
		typeDef, err := lib.semantic(*ref.Named)
		if err != nil {
			return Value{}, err
		}
		return d.decodeType(lib, typeDef, path)
	case ref.Extern != nil:
		external, err := d.registry.library(ref.Extern.LibID)
		if err != nil {
			return Value{}, err
		}
		typeDef, err := external.semantic(ref.Extern.SemID)
		if err != nil {
			return Value{}, err
		}
		return d.decodeType(external, typeDef, path)
	case ref.Inline != nil:
		return d.decodeType(lib, ref.Inline, path)
	default:
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
}

func (d *decoder) resolveRef(lib *typeLibrary, ref typeRef) (*typeLibrary, *parsedType, error) {
	switch {
	case ref.Named != nil:
		typeDef, err := lib.semantic(*ref.Named)
		return lib, typeDef, err
	case ref.Extern != nil:
		external, err := d.registry.library(ref.Extern.LibID)
		if err != nil {
			return nil, nil, err
		}
		typeDef, err := external.semantic(ref.Extern.SemID)
		return external, typeDef, err
	default:
		return lib, ref.Inline, nil
	}
}

func (d *decoder) decodePrimitive(primitive uint8, path string) (Value, error) {
	if primitive == 0 {
		return Value{Kind: ValueUnit, Primitive: primitive}, nil
	}
	size := primitiveSize(primitive)
	if size == 0 {
		return Value{}, fmt.Errorf("%s: reserved primitive 0x%02x", path, primitive)
	}
	raw, err := d.strict.Raw(uint64(size))
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	value := Value{Kind: ValueNumber, Primitive: primitive, Raw: raw}
	class := primitive & 0xc0
	if primitive == 0x40 {
		class = 0
	}
	if size <= 8 && (class == 0 || class == 0x80) {
		unsigned := littleEndianUint(raw)
		if class == 0x80 && unsigned == 0 {
			return Value{}, fmt.Errorf("%s: non-zero primitive contains zero", path)
		}
		value.Unsigned = &unsigned
	} else if size <= 8 && class == 0x40 {
		signed := littleEndianInt(raw)
		value.Signed = &signed
	}
	return value, nil
}

func primitiveSize(primitive uint8) int {
	if primitive == 0 {
		return 0
	}
	if primitive == 0x40 {
		return 1
	}
	if primitive == 0xc0 {
		return 2
	}
	code := primitive & 0x3f
	if code&0x20 == 0 {
		return int(code & 0x1f)
	}
	return 16 * (2 + int(code&0x1f))
}

func littleEndianUint(raw []byte) uint64 {
	var buf [8]byte
	copy(buf[:], raw)
	return binary.LittleEndian.Uint64(buf[:])
}

func littleEndianInt(raw []byte) int64 {
	unsigned := littleEndianUint(raw)
	bits := uint(len(raw) * 8)
	if bits < 64 && unsigned&(uint64(1)<<(bits-1)) != 0 {
		unsigned |= math.MaxUint64 << bits
	}
	return int64(unsigned)
}

func (d *decoder) decodeUnicode(path string) (Value, error) {
	first, err := d.strict.U8()
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	width := 1
	switch {
	case first < utf8.RuneSelf:
	case first&0xe0 == 0xc0:
		width = 2
	case first&0xf0 == 0xe0:
		width = 3
	case first&0xf8 == 0xf0:
		width = 4
	default:
		return Value{}, fmt.Errorf("%s: invalid UTF-8", path)
	}
	raw := []byte{first}
	if width > 1 {
		rest, err := d.strict.Raw(uint64(width - 1))
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		raw = append(raw, rest...)
	}
	if !utf8.Valid(raw) {
		return Value{}, fmt.Errorf("%s: invalid UTF-8", path)
	}
	return Value{Kind: ValueString, Text: string(raw), Raw: raw}, nil
}

func parseVariant(value string) (string, uint8, error) {
	name, tagText, ok := strings.Cut(value, ":")
	if !ok || name == "" {
		return "", 0, ErrMalformedType
	}
	tag, err := strconv.ParseUint(tagText, 10, 8)
	if err != nil {
		return "", 0, ErrMalformedType
	}
	return name, uint8(tag), nil
}

func (d *decoder) decodeEnum(variants []parsedVariant, path string) (Value, error) {
	tag, err := d.strict.U8()
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	for _, variant := range variants {
		if variant.tag == tag {
			return Value{Kind: ValueEnum, Name: variant.name, Text: variant.name, Tag: tag}, nil
		}
	}
	return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, tag)
}

func (d *decoder) decodeUnion(lib *typeLibrary, variants []parsedVariant, path string) (Value, error) {
	tag, err := d.strict.U8()
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	for _, variant := range variants {
		if variant.tag == tag {
			inner, err := d.decodeRef(lib, variant.ref, path+"."+variant.name)
			if err != nil {
				return Value{}, err
			}
			return Value{Kind: ValueUnion, Name: variant.name, Tag: tag, Inner: &inner}, nil
		}
	}
	return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, tag)
}

func (d *decoder) decodeTuple(lib *typeLibrary, refs []typeRef, path string) (Value, error) {
	if len(refs) == 2 {
		if restLib, restRef, limits, ok := d.restrictedString(lib, refs); ok {
			length, err := d.strict.Length(limits.Max)
			if err != nil || length < limits.Min {
				if err == nil {
					err = strict.ErrOutOfBounds
				}
				return Value{}, fmt.Errorf("%s: %w", path, err)
			}
			raw, err := d.strict.Raw(length)
			if err != nil {
				return Value{}, fmt.Errorf("%s: %w", path, err)
			}
			if len(raw) == 0 || !d.refEnumHasTag(lib, refs[0], raw[0]) {
				return Value{}, fmt.Errorf("%s: %w", path, ErrUnknownVariant)
			}
			for _, char := range raw[1:] {
				if !d.refEnumHasTag(restLib, restRef, char) {
					return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, char)
				}
			}
			return Value{Kind: ValueString, Text: string(raw), Raw: raw}, nil
		}
	}
	items := make([]Value, 0, len(refs))
	for index, ref := range refs {
		item, err := d.decodeRef(lib, ref, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return Value{}, err
		}
		items = append(items, item)
	}
	return Value{Kind: ValueTuple, Items: items}, nil
}

func (d *decoder) restrictedString(lib *typeLibrary, refs []typeRef) (*typeLibrary, typeRef, sizing, bool) {
	if len(refs) != 2 || !d.refIsCharEnum(lib, refs[0]) {
		return nil, typeRef{}, sizing{}, false
	}
	restLib, restType, err := d.resolveRef(lib, refs[1])
	if err != nil || restType == nil || restType.kind != "List" || !d.refIsCharEnum(restLib, restType.refs[0]) || restType.limits.Max == math.MaxUint64 {
		return nil, typeRef{}, sizing{}, false
	}
	limits := restType.limits
	limits.Min++
	limits.Max++
	return restLib, restType.refs[0], limits, true
}

func (d *decoder) decodeStruct(lib *typeLibrary, fields []parsedField, path string) (Value, error) {
	result := make([]Field, 0, len(fields))
	for _, field := range fields {
		value, err := d.decodeRef(lib, field.Type, path+"."+field.Name)
		if err != nil {
			return Value{}, err
		}
		result = append(result, Field{Name: field.Name, Value: value})
	}
	return Value{Kind: ValueStruct, Fields: result}, nil
}

func (d *decoder) decodeArray(lib *typeLibrary, ref typeRef, length uint16, path string) (Value, error) {
	if d.refIsPrimitive(lib, ref, 0x40) {
		raw, err := d.strict.Raw(uint64(length))
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		return Value{Kind: ValueBytes, Raw: raw}, nil
	}
	items := make([]Value, 0, length)
	for index := uint16(0); index < length; index++ {
		value, err := d.decodeRef(lib, ref, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return Value{}, err
		}
		items = append(items, value)
	}
	return Value{Kind: ValueList, Items: items}, nil
}

func (d *decoder) decodeList(lib *typeLibrary, kind string, ref typeRef, limits sizing, path string) (Value, error) {
	length, err := d.strict.Length(limits.Max)
	if err != nil || length < limits.Min {
		if err == nil {
			err = strict.ErrOutOfBounds
		}
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	valueKind := ValueList
	if kind == "Set" {
		valueKind = ValueSet
	}
	if kind == "List" && d.refIsPrimitive(lib, ref, 0x40) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		return Value{Kind: ValueBytes, Raw: raw}, nil
	}
	if kind == "List" && d.refIsUnicode(lib, ref) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		if !utf8.Valid(raw) {
			return Value{}, fmt.Errorf("%s: invalid UTF-8", path)
		}
		return Value{Kind: ValueString, Text: string(raw), Raw: raw}, nil
	}
	if kind == "List" && d.refIsCharEnum(lib, ref) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		for _, char := range raw {
			if !d.refEnumHasTag(lib, ref, char) {
				return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, char)
			}
		}
		return Value{Kind: ValueString, Text: string(raw), Raw: raw}, nil
	}
	items := make([]Value, 0, int(length))
	seen := make(map[string]struct{})
	var previous *Value
	for index := uint64(0); index < length; index++ {
		start := d.reader.Len()
		value, err := d.decodeRef(lib, ref, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return Value{}, err
		}
		if valueKind == ValueSet {
			consumed := start - d.reader.Len()
			encoded := make([]byte, consumed)
			position := int64(d.reader.Size()) - int64(start)
			if _, err := d.reader.ReadAt(encoded, position); err != nil {
				return Value{}, err
			}
			key := string(encoded)
			if _, duplicate := seen[key]; duplicate {
				return Value{}, fmt.Errorf("%s: %w duplicate set item", path, ErrNonCanonical)
			}
			if previous != nil {
				order, comparable := compareValue(*previous, value)
				if !comparable || order >= 0 {
					return Value{}, fmt.Errorf("%s: %w unordered set item", path, ErrNonCanonical)
				}
			}
			seen[key] = struct{}{}
			copy := value
			previous = &copy
		}
		items = append(items, value)
	}
	return Value{Kind: valueKind, Items: items}, nil
}

func (d *decoder) decodeMap(lib *typeLibrary, refs []typeRef, limits sizing, path string) (Value, error) {
	length, err := d.strict.Length(limits.Max)
	if err != nil || length < limits.Min {
		if err == nil {
			err = strict.ErrOutOfBounds
		}
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	entries := make([]Entry, 0, int(length))
	seen := make(map[string]struct{})
	var previous *Value
	for index := uint64(0); index < length; index++ {
		start := d.reader.Len()
		key, err := d.decodeRef(lib, refs[0], fmt.Sprintf("%s[%d].key", path, index))
		if err != nil {
			return Value{}, err
		}
		consumed := start - d.reader.Len()
		encoded := make([]byte, consumed)
		position := int64(d.reader.Size()) - int64(start)
		if _, err := d.reader.ReadAt(encoded, position); err != nil {
			return Value{}, err
		}
		encodedKey := string(encoded)
		if _, duplicate := seen[encodedKey]; duplicate {
			return Value{}, fmt.Errorf("%s: %w duplicate map key", path, ErrNonCanonical)
		}
		if previous != nil {
			order, comparable := compareValue(*previous, key)
			if !comparable || order >= 0 {
				return Value{}, fmt.Errorf("%s: %w unordered map key", path, ErrNonCanonical)
			}
		}
		seen[encodedKey] = struct{}{}
		copy := key
		previous = &copy
		value, err := d.decodeRef(lib, refs[1], fmt.Sprintf("%s[%d].value", path, index))
		if err != nil {
			return Value{}, err
		}
		entries = append(entries, Entry{Key: key, Value: value})
	}
	return Value{Kind: ValueMap, Entries: entries}, nil
}

func (d *decoder) refIsPrimitive(lib *typeLibrary, ref typeRef, primitive uint8) bool {
	_, typeDef, err := d.resolveRef(lib, ref)
	return err == nil && typeDef != nil && typeDef.kind == "Primitive" && typeDef.primitive == primitive
}

func (d *decoder) refIsUnicode(lib *typeLibrary, ref typeRef) bool {
	_, typeDef, err := d.resolveRef(lib, ref)
	return err == nil && typeDef != nil && (typeDef.kind == "UnicodeChar" || typeDef.kind == "Unicode")
}

func (d *decoder) refIsCharEnum(lib *typeLibrary, ref typeRef) bool {
	resolvedLib, typeDef, err := d.resolveRef(lib, ref)
	if err != nil || typeDef == nil {
		return false
	}
	if typeDef.kind == "Enum" {
		if len(typeDef.variants) == 0 {
			return false
		}
		for _, variant := range typeDef.variants {
			if variant.tag < 32 || variant.tag > 127 {
				return false
			}
		}
		return true
	}
	return typeDef.kind == "Tuple" && len(typeDef.refs) > 0 && d.refIsCharEnum(resolvedLib, typeDef.refs[0])
}

func (d *decoder) refEnumHasTag(lib *typeLibrary, ref typeRef, wanted uint8) bool {
	resolvedLib, typeDef, err := d.resolveRef(lib, ref)
	if err != nil || typeDef == nil {
		return false
	}
	if typeDef.kind == "Enum" {
		for _, variant := range typeDef.variants {
			if variant.tag == wanted {
				return true
			}
		}
		return false
	}
	return typeDef.kind == "Tuple" && len(typeDef.refs) > 0 && d.refEnumHasTag(resolvedLib, typeDef.refs[0], wanted)
}
