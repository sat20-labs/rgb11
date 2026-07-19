package strict_types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

// Upstream-Repository: rgb-protocol/rgb-strict-types
// Upstream-Version: 1.0.2
// Upstream-Commit: b441bc508a9fcb556e243c7a9c48f42d4582e32e
// Upstream-File: src/value/decode.rs
// Upstream-File-SHA256: f64e809fe99b766a3489837f0a844ff9c67ae6174cd813084fb1259f6a79d19f
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
	Inline json.RawMessage
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

func (d *decoder) decodeType(lib *typeLibrary, raw json.RawMessage, path string) (Value, error) {
	start := d.reader.Len()
	value, err := d.decodeTypeBody(lib, raw, path)
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

func (d *decoder) decodeTypeBody(lib *typeLibrary, raw json.RawMessage, path string) (Value, error) {
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(raw, &variants); err != nil || len(variants) != 1 {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	for kind, body := range variants {
		switch kind {
		case "Primitive":
			var primitive uint8
			if err := json.Unmarshal(body, &primitive); err != nil {
				return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
			}
			return d.decodePrimitive(primitive, path)
		case "UnicodeChar", "Unicode":
			return d.decodeUnicode(path)
		case "Enum":
			return d.decodeEnum(body, path)
		case "Union":
			return d.decodeUnion(lib, body, path)
		case "Tuple":
			return d.decodeTuple(lib, body, path)
		case "Struct":
			return d.decodeStruct(lib, body, path)
		case "Array":
			return d.decodeArray(lib, body, path)
		case "List", "Set":
			return d.decodeList(lib, kind, body, path)
		case "Map":
			return d.decodeMap(lib, body, path)
		default:
			return Value{}, fmt.Errorf("%s: %w %q", path, ErrMalformedType, kind)
		}
	}
	panic("unreachable")
}

func (d *decoder) decodeRef(lib *typeLibrary, raw json.RawMessage, path string) (Value, error) {
	ref, err := parseRef(raw)
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
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

func parseRef(raw json.RawMessage) (typeRef, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || len(object) != 1 {
		return typeRef{}, ErrMalformedType
	}
	var result typeRef
	if value, ok := object["named"]; ok {
		var semID string
		if err := json.Unmarshal(value, &semID); err != nil || len(semID) != 64 {
			return typeRef{}, ErrMalformedType
		}
		result.Named = &semID
		return result, nil
	}
	if value, ok := object["extern"]; ok {
		var external struct {
			LibID string `json:"libId"`
			SemID string `json:"semId"`
		}
		if err := json.Unmarshal(value, &external); err != nil || len(external.LibID) != 64 || len(external.SemID) != 64 {
			return typeRef{}, ErrMalformedType
		}
		result.Extern = &external
		return result, nil
	}
	if value, ok := object["inline"]; ok {
		result.Inline = value
		return result, nil
	}
	return typeRef{}, ErrMalformedType
}

func (d *decoder) resolveRef(lib *typeLibrary, raw json.RawMessage) (*typeLibrary, json.RawMessage, error) {
	ref, err := parseRef(raw)
	if err != nil {
		return nil, nil, err
	}
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

func (d *decoder) decodeEnum(body json.RawMessage, path string) (Value, error) {
	var variants []string
	if err := json.Unmarshal(body, &variants); err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	tag, err := d.strict.U8()
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	for _, variant := range variants {
		name, candidate, err := parseVariant(variant)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		if candidate == tag {
			return Value{Kind: ValueEnum, Name: name, Text: name, Tag: tag}, nil
		}
	}
	return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, tag)
}

func (d *decoder) decodeUnion(lib *typeLibrary, body json.RawMessage, path string) (Value, error) {
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(body, &variants); err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	tag, err := d.strict.U8()
	if err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, err)
	}
	for variant, ref := range variants {
		name, candidate, err := parseVariant(variant)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		if candidate == tag {
			inner, err := d.decodeRef(lib, ref, path+"."+name)
			if err != nil {
				return Value{}, err
			}
			return Value{Kind: ValueUnion, Name: name, Tag: tag, Inner: &inner}, nil
		}
	}
	return Value{}, fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, tag)
}

func (d *decoder) decodeTuple(lib *typeLibrary, body json.RawMessage, path string) (Value, error) {
	var refs []json.RawMessage
	if err := json.Unmarshal(body, &refs); err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
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

func (d *decoder) restrictedString(lib *typeLibrary, refs []json.RawMessage) (*typeLibrary, json.RawMessage, sizing, bool) {
	if len(refs) != 2 || !d.refIsCharEnum(lib, refs[0]) {
		return nil, nil, sizing{}, false
	}
	restLib, restType, err := d.resolveRef(lib, refs[1])
	if err != nil {
		return nil, nil, sizing{}, false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(restType, &object) != nil {
		return nil, nil, sizing{}, false
	}
	var parts []json.RawMessage
	if json.Unmarshal(object["List"], &parts) != nil || len(parts) != 2 || !d.refIsCharEnum(restLib, parts[0]) {
		return nil, nil, sizing{}, false
	}
	var limits sizing
	if json.Unmarshal(parts[1], &limits) != nil || limits.Max == math.MaxUint64 {
		return nil, nil, sizing{}, false
	}
	limits.Min++
	limits.Max++
	return restLib, parts[0], limits, true
}

func (d *decoder) decodeStruct(lib *typeLibrary, body json.RawMessage, path string) (Value, error) {
	var fields []struct {
		Name string          `json:"name"`
		Type json.RawMessage `json:"ty"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
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

func (d *decoder) decodeArray(lib *typeLibrary, body json.RawMessage, path string) (Value, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(body, &parts); err != nil || len(parts) != 2 {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var length uint16
	if err := json.Unmarshal(parts[1], &length); err != nil {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	if d.refIsPrimitive(lib, parts[0], 0x40) {
		raw, err := d.strict.Raw(uint64(length))
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		return Value{Kind: ValueBytes, Raw: raw}, nil
	}
	items := make([]Value, 0, length)
	for index := uint16(0); index < length; index++ {
		value, err := d.decodeRef(lib, parts[0], fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return Value{}, err
		}
		items = append(items, value)
	}
	return Value{Kind: ValueList, Items: items}, nil
}

func (d *decoder) decodeList(lib *typeLibrary, kind string, body json.RawMessage, path string) (Value, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(body, &parts); err != nil || len(parts) != 2 {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var limits sizing
	if err := json.Unmarshal(parts[1], &limits); err != nil || limits.Min > limits.Max {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
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
	if kind == "List" && d.refIsPrimitive(lib, parts[0], 0x40) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		return Value{Kind: ValueBytes, Raw: raw}, nil
	}
	if kind == "List" && d.refIsUnicode(lib, parts[0]) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		if !utf8.Valid(raw) {
			return Value{}, fmt.Errorf("%s: invalid UTF-8", path)
		}
		return Value{Kind: ValueString, Text: string(raw), Raw: raw}, nil
	}
	if kind == "List" && d.refIsCharEnum(lib, parts[0]) {
		raw, err := d.strict.Raw(length)
		if err != nil {
			return Value{}, fmt.Errorf("%s: %w", path, err)
		}
		for _, char := range raw {
			if !d.refEnumHasTag(lib, parts[0], char) {
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
		value, err := d.decodeRef(lib, parts[0], fmt.Sprintf("%s[%d]", path, index))
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

func (d *decoder) decodeMap(lib *typeLibrary, body json.RawMessage, path string) (Value, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(body, &parts); err != nil || len(parts) != 3 {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var limits sizing
	if err := json.Unmarshal(parts[2], &limits); err != nil || limits.Min > limits.Max {
		return Value{}, fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
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
		key, err := d.decodeRef(lib, parts[0], fmt.Sprintf("%s[%d].key", path, index))
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
		value, err := d.decodeRef(lib, parts[1], fmt.Sprintf("%s[%d].value", path, index))
		if err != nil {
			return Value{}, err
		}
		entries = append(entries, Entry{Key: key, Value: value})
	}
	return Value{Kind: ValueMap, Entries: entries}, nil
}

func (d *decoder) refIsPrimitive(lib *typeLibrary, raw json.RawMessage, primitive uint8) bool {
	resolvedLib, typeDef, err := d.resolveRef(lib, raw)
	if err != nil || resolvedLib == nil {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(typeDef, &object) != nil {
		return false
	}
	var code uint8
	return json.Unmarshal(object["Primitive"], &code) == nil && code == primitive
}

func (d *decoder) refIsUnicode(lib *typeLibrary, raw json.RawMessage) bool {
	_, typeDef, err := d.resolveRef(lib, raw)
	if err != nil {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(typeDef, &object) != nil {
		return false
	}
	_, unicodeChar := object["UnicodeChar"]
	_, unicode := object["Unicode"]
	return unicodeChar || unicode
}

func (d *decoder) refIsCharEnum(lib *typeLibrary, raw json.RawMessage) bool {
	resolvedLib, typeDef, err := d.resolveRef(lib, raw)
	if err != nil {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(typeDef, &object) != nil {
		return false
	}
	if enumBody, ok := object["Enum"]; ok {
		var variants []string
		if json.Unmarshal(enumBody, &variants) != nil || len(variants) == 0 {
			return false
		}
		for _, variant := range variants {
			_, tag, err := parseVariant(variant)
			if err != nil || tag < 32 || tag > 127 {
				return false
			}
		}
		return true
	}
	if tupleBody, ok := object["Tuple"]; ok {
		var refs []json.RawMessage
		return json.Unmarshal(tupleBody, &refs) == nil && len(refs) > 0 && d.refIsCharEnum(resolvedLib, refs[0])
	}
	return false
}

func (d *decoder) refEnumHasTag(lib *typeLibrary, raw json.RawMessage, wanted uint8) bool {
	resolvedLib, typeDef, err := d.resolveRef(lib, raw)
	if err != nil {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(typeDef, &object) != nil {
		return false
	}
	if enumBody, ok := object["Enum"]; ok {
		var variants []string
		if json.Unmarshal(enumBody, &variants) != nil {
			return false
		}
		for _, variant := range variants {
			_, tag, err := parseVariant(variant)
			if err == nil && tag == wanted {
				return true
			}
		}
		return false
	}
	if tupleBody, ok := object["Tuple"]; ok {
		var refs []json.RawMessage
		return json.Unmarshal(tupleBody, &refs) == nil && len(refs) > 0 && d.refEnumHasTag(resolvedLib, refs[0], wanted)
	}
	return false
}
