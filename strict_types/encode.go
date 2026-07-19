package strict_types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

type encoder struct {
	registry *Registry
	buffer   bytes.Buffer
	strict   *strict.Encoder
}

// Encode is the inverse of Decode and is used by the wallet-side operation
// builder. Values are checked against the frozen strict type definition; stale
// Value.Encoded data is never trusted.
func (r *Registry) Encode(libraryName, typeName string, value Value) ([]byte, error) {
	lib, err := r.library(libraryName)
	if err != nil {
		return nil, err
	}
	typeDef, err := lib.named(typeName)
	if err != nil {
		return nil, err
	}
	e := &encoder{registry: r}
	e.strict = strict.NewEncoder(&e.buffer)
	if err := e.encodeType(lib, typeDef, value, libraryName+"."+typeName); err != nil {
		return nil, err
	}
	return e.buffer.Bytes(), nil
}

func (e *encoder) encodeType(lib *typeLibrary, raw json.RawMessage, value Value, path string) error {
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(raw, &variants); err != nil || len(variants) != 1 {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	for kind, body := range variants {
		switch kind {
		case "Primitive":
			var primitive uint8
			if json.Unmarshal(body, &primitive) != nil {
				return fmt.Errorf("%s: %w", path, ErrMalformedType)
			}
			return e.encodePrimitive(primitive, value, path)
		case "UnicodeChar", "Unicode":
			return e.encodeUnicode(value, path)
		case "Enum":
			return e.encodeEnum(body, value, path)
		case "Union":
			return e.encodeUnion(lib, body, value, path)
		case "Tuple":
			return e.encodeTuple(lib, body, value, path)
		case "Struct":
			return e.encodeStruct(lib, body, value, path)
		case "Array":
			return e.encodeArray(lib, body, value, path)
		case "List", "Set":
			return e.encodeList(lib, kind, body, value, path)
		case "Map":
			return e.encodeMap(lib, body, value, path)
		default:
			return fmt.Errorf("%s: %w %q", path, ErrMalformedType, kind)
		}
	}
	panic("unreachable")
}

func (e *encoder) encodeRef(lib *typeLibrary, raw json.RawMessage, value Value, path string) error {
	ref, err := parseRef(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case ref.Named != nil:
		typeDef, err := lib.semantic(*ref.Named)
		if err != nil {
			return err
		}
		return e.encodeType(lib, typeDef, value, path)
	case ref.Extern != nil:
		external, err := e.registry.library(ref.Extern.LibID)
		if err != nil {
			return err
		}
		typeDef, err := external.semantic(ref.Extern.SemID)
		if err != nil {
			return err
		}
		return e.encodeType(external, typeDef, value, path)
	case ref.Inline != nil:
		return e.encodeType(lib, ref.Inline, value, path)
	default:
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
}

func (e *encoder) encodePrimitive(primitive uint8, value Value, path string) error {
	value = value.Unwrap()
	if primitive == 0 {
		if value.Kind != ValueUnit {
			return fmt.Errorf("%s: %w primitive unit", path, ErrMalformedType)
		}
		return nil
	}
	size := primitiveSize(primitive)
	if size == 0 || value.Kind != ValueNumber {
		return fmt.Errorf("%s: %w primitive", path, ErrMalformedType)
	}
	raw := append([]byte(nil), value.Raw...)
	if len(raw) == 0 && size <= 8 {
		raw = make([]byte, size)
		if value.Unsigned != nil {
			var encoded [8]byte
			binary.LittleEndian.PutUint64(encoded[:], *value.Unsigned)
			copy(raw, encoded[:])
		} else if value.Signed != nil {
			var encoded [8]byte
			binary.LittleEndian.PutUint64(encoded[:], uint64(*value.Signed))
			copy(raw, encoded[:])
		}
	}
	if len(raw) != size {
		return fmt.Errorf("%s: %w primitive width", path, ErrMalformedType)
	}
	return e.strict.Raw(raw)
}

func (e *encoder) encodeUnicode(value Value, path string) error {
	value = value.Unwrap()
	raw := value.Raw
	if len(raw) == 0 {
		raw = []byte(value.Text)
	}
	if value.Kind != ValueString || !utf8.Valid(raw) || utf8.RuneCount(raw) != 1 {
		return fmt.Errorf("%s: invalid UTF-8 character", path)
	}
	return e.strict.Raw(raw)
}

func (e *encoder) encodeEnum(body json.RawMessage, value Value, path string) error {
	value = value.Unwrap()
	if value.Kind != ValueEnum {
		return fmt.Errorf("%s: %w enum", path, ErrMalformedType)
	}
	var variants []string
	if json.Unmarshal(body, &variants) != nil {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	for _, variant := range variants {
		name, tag, err := parseVariant(variant)
		if err == nil && tag == value.Tag && (value.Name == "" || value.Name == name) {
			return e.strict.U8(tag)
		}
	}
	return fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, value.Tag)
}

func (e *encoder) encodeUnion(lib *typeLibrary, body json.RawMessage, value Value, path string) error {
	value = value.Unwrap()
	if value.Kind != ValueUnion || value.Inner == nil {
		return fmt.Errorf("%s: %w union", path, ErrMalformedType)
	}
	var variants map[string]json.RawMessage
	if json.Unmarshal(body, &variants) != nil {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	for variant, ref := range variants {
		name, tag, err := parseVariant(variant)
		if err == nil && tag == value.Tag && name == value.Name {
			if err := e.strict.U8(tag); err != nil {
				return err
			}
			return e.encodeRef(lib, ref, *value.Inner, path+"."+name)
		}
	}
	return fmt.Errorf("%s: %w: %d", path, ErrUnknownVariant, value.Tag)
}

func (e *encoder) encodeTuple(lib *typeLibrary, body json.RawMessage, value Value, path string) error {
	var refs []json.RawMessage
	if json.Unmarshal(body, &refs) != nil {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	decoderView := &decoder{registry: e.registry}
	if len(refs) == 2 {
		if _, _, limits, ok := decoderView.restrictedString(lib, refs); ok {
			if value.Kind != ValueString {
				return fmt.Errorf("%s: %w restricted string", path, ErrMalformedType)
			}
			raw := value.Raw
			if len(raw) == 0 {
				raw = []byte(value.Text)
			}
			if uint64(len(raw)) < limits.Min || uint64(len(raw)) > limits.Max {
				return strict.ErrOutOfBounds
			}
			if err := e.strict.Length(uint64(len(raw)), limits.Max); err != nil {
				return err
			}
			return e.strict.Raw(raw)
		}
	}
	if value.Kind == ValueTuple && len(value.Items) == len(refs) {
		for index, ref := range refs {
			if err := e.encodeRef(lib, ref, value.Items[index], fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	}
	if len(refs) == 1 {
		return e.encodeRef(lib, refs[0], value.Unwrap(), path+"[0]")
	}
	if value.Kind != ValueTuple || len(value.Items) != len(refs) {
		return fmt.Errorf("%s: %w tuple", path, ErrMalformedType)
	}
	return fmt.Errorf("%s: %w tuple", path, ErrMalformedType)
}

func (e *encoder) encodeStruct(lib *typeLibrary, body json.RawMessage, value Value, path string) error {
	var fields []struct {
		Name string          `json:"name"`
		Type json.RawMessage `json:"ty"`
	}
	if json.Unmarshal(body, &fields) != nil {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	value = value.Unwrap()
	if value.Kind != ValueStruct || len(value.Fields) != len(fields) {
		return fmt.Errorf("%s: %w struct", path, ErrMalformedType)
	}
	for index, field := range fields {
		if value.Fields[index].Name != field.Name {
			return fmt.Errorf("%s: %w field %s", path, ErrMalformedType, field.Name)
		}
		if err := e.encodeRef(lib, field.Type, value.Fields[index].Value, path+"."+field.Name); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) encodeArray(lib *typeLibrary, body json.RawMessage, value Value, path string) error {
	var parts []json.RawMessage
	if json.Unmarshal(body, &parts) != nil || len(parts) != 2 {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var length uint16
	if json.Unmarshal(parts[1], &length) != nil {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	decoderView := &decoder{registry: e.registry}
	if decoderView.refIsPrimitive(lib, parts[0], 0x40) {
		if value.Kind != ValueBytes || len(value.Raw) != int(length) {
			return fmt.Errorf("%s: %w byte array", path, ErrMalformedType)
		}
		return e.strict.Raw(value.Raw)
	}
	value = value.Unwrap()
	if value.Kind != ValueList || len(value.Items) != int(length) {
		return fmt.Errorf("%s: %w array", path, ErrMalformedType)
	}
	for index, item := range value.Items {
		if err := e.encodeRef(lib, parts[0], item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) encodeList(lib *typeLibrary, kind string, body json.RawMessage, value Value, path string) error {
	var parts []json.RawMessage
	if json.Unmarshal(body, &parts) != nil || len(parts) != 2 {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var limits sizing
	if json.Unmarshal(parts[1], &limits) != nil || limits.Min > limits.Max {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	decoderView := &decoder{registry: e.registry}
	if kind == "List" && (decoderView.refIsPrimitive(lib, parts[0], 0x40) || decoderView.refIsUnicode(lib, parts[0]) || decoderView.refIsCharEnum(lib, parts[0])) {
		raw := value.Raw
		if len(raw) == 0 && value.Kind == ValueString {
			raw = []byte(value.Text)
		}
		if (value.Kind != ValueBytes && value.Kind != ValueString) || uint64(len(raw)) < limits.Min || uint64(len(raw)) > limits.Max {
			return fmt.Errorf("%s: %w confined bytes", path, strict.ErrOutOfBounds)
		}
		if err := e.strict.Length(uint64(len(raw)), limits.Max); err != nil {
			return err
		}
		return e.strict.Raw(raw)
	}
	value = value.Unwrap()
	wantKind := ValueList
	if kind == "Set" {
		wantKind = ValueSet
	}
	if value.Kind != wantKind || uint64(len(value.Items)) < limits.Min || uint64(len(value.Items)) > limits.Max {
		return fmt.Errorf("%s: %w collection", path, strict.ErrOutOfBounds)
	}
	if wantKind == ValueSet {
		for index := 1; index < len(value.Items); index++ {
			order, ok := compareValue(value.Items[index-1], value.Items[index])
			if !ok || order >= 0 {
				return fmt.Errorf("%s: %w unordered set", path, ErrNonCanonical)
			}
		}
	}
	if err := e.strict.Length(uint64(len(value.Items)), limits.Max); err != nil {
		return err
	}
	for index, item := range value.Items {
		if err := e.encodeRef(lib, parts[0], item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) encodeMap(lib *typeLibrary, body json.RawMessage, value Value, path string) error {
	var parts []json.RawMessage
	if json.Unmarshal(body, &parts) != nil || len(parts) != 3 {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	var limits sizing
	if json.Unmarshal(parts[2], &limits) != nil || limits.Min > limits.Max {
		return fmt.Errorf("%s: %w", path, ErrMalformedType)
	}
	value = value.Unwrap()
	if value.Kind != ValueMap || uint64(len(value.Entries)) < limits.Min || uint64(len(value.Entries)) > limits.Max {
		return fmt.Errorf("%s: %w map", path, strict.ErrOutOfBounds)
	}
	for index := 1; index < len(value.Entries); index++ {
		order, ok := compareValue(value.Entries[index-1].Key, value.Entries[index].Key)
		if !ok || order >= 0 {
			return fmt.Errorf("%s: %w unordered map", path, ErrNonCanonical)
		}
	}
	if err := e.strict.Length(uint64(len(value.Entries)), limits.Max); err != nil {
		return err
	}
	for index, entry := range value.Entries {
		if err := e.encodeRef(lib, parts[0], entry.Key, fmt.Sprintf("%s[%d].key", path, index)); err != nil {
			return err
		}
		if err := e.encodeRef(lib, parts[1], entry.Value, fmt.Sprintf("%s[%d].value", path, index)); err != nil {
			return err
		}
	}
	return nil
}
