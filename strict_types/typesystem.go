package strict_types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

// Upstream-Repository: rgb-protocol/rgb-strict-types
// Upstream-Version: 1.0.2
// Upstream-Commit: b441bc508a9fcb556e243c7a9c48f42d4582e32e
// Upstream-File: src/typesys/system.rs
// Translation-Revision: 1

var ErrInvalidTypeSystem = errors.New("invalid strict type system")

// AddTypeSystem installs a closed TypeSystem decoded with the StrictTypes
// library. Contract consignments carry such a system so their semantic state
// values can be decoded without trusting a locally invented schema.
func (r *Registry) AddTypeSystem(name string, value Value) error {
	if r == nil || name == "" {
		return ErrInvalidTypeSystem
	}
	entries, ok := valueMap(value)
	if !ok || len(entries) == 0 {
		return ErrInvalidTypeSystem
	}
	lib := &typeLibrary{
		ID:          "typesystem:" + name,
		Name:        name,
		bySemID:     make(map[string]json.RawMessage, len(entries)),
		byName:      make(map[string]json.RawMessage),
		semIDByName: make(map[string]string),
	}
	for _, entry := range entries {
		semID, ok := semanticID(entry.Key)
		if !ok {
			return ErrInvalidTypeSystem
		}
		definition, err := typeDefinition(entry.Value)
		if err != nil {
			kind := entry.Value.Unwrap().Name
			return fmt.Errorf("%w: %s (%s): %v", ErrInvalidTypeSystem, semID, kind, err)
		}
		encoded, err := json.Marshal(definition)
		if err != nil {
			return err
		}
		lib.bySemID[semID] = encoded
	}
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("duplicate strict type system %s", name)
	}
	r.byName[name] = lib
	r.byID[lib.ID] = lib
	return nil
}

// DecodeSemantic decodes a value using a semantic type identifier from a
// previously added TypeSystem.
func (r *Registry) DecodeSemantic(typeSystem, semID string, data []byte) (Value, error) {
	lib, err := r.library(typeSystem)
	if err != nil {
		return Value{}, err
	}
	typeDef, err := lib.semantic(semID)
	if err != nil {
		return Value{}, err
	}
	reader := bytes.NewReader(data)
	decoder := &decoder{registry: r, reader: reader, strict: strict.NewDecoder(reader)}
	value, err := decoder.decodeType(lib, typeDef, typeSystem+":"+semID)
	if err != nil {
		return Value{}, err
	}
	if reader.Len() != 0 {
		return Value{}, strict.ErrTrailingData
	}
	return value, nil
}

// EncodeSemantic is the inverse of DecodeSemantic. It is used by the wallet
// issuer to construct global and structured state from the exact TypeSystem
// embedded in the selected standard-schema template.
func (r *Registry) EncodeSemantic(typeSystem, semID string, value Value) ([]byte, error) {
	lib, err := r.library(typeSystem)
	if err != nil {
		return nil, err
	}
	typeDef, err := lib.semantic(semID)
	if err != nil {
		return nil, err
	}
	encoder := &encoder{registry: r}
	encoder.strict = strict.NewEncoder(&encoder.buffer)
	if err := encoder.encodeType(lib, typeDef, value, typeSystem+":"+semID); err != nil {
		return nil, err
	}
	return encoder.buffer.Bytes(), nil
}

// SemanticTypeDefinition returns the normalized runtime definition for
// diagnostics and deterministic cross-language test vectors.
func (r *Registry) SemanticTypeDefinition(typeSystem, semID string) ([]byte, error) {
	lib, err := r.library(typeSystem)
	if err != nil {
		return nil, err
	}
	typeDef, err := lib.semantic(semID)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), typeDef...), nil
}

func typeDefinition(value Value) (map[string]any, error) {
	value = value.Unwrap()
	if value.Kind != ValueUnion || value.Inner == nil {
		return nil, ErrInvalidTypeSystem
	}
	data := *value.Inner
	switch value.Name {
	case "primitive":
		items, ok := valueItems(data, 1)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		primitive := items[0].Unwrap()
		code, ok := primitive.Uint64()
		if !ok || code > 255 {
			return nil, ErrInvalidTypeSystem
		}
		return map[string]any{"Primitive": code}, nil
	case "unicode":
		return map[string]any{"UnicodeChar": nil}, nil
	case "enum":
		items, ok := valueItems(data, 1)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		variants, ok := valueSequence(items[0])
		if !ok || len(variants) == 0 {
			return nil, ErrInvalidTypeSystem
		}
		result := make([]string, 0, len(variants))
		for _, variant := range variants {
			name, tag, ok := namedTag(variant)
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			result = append(result, fmt.Sprintf("%s:%d", name, tag))
		}
		return map[string]any{"Enum": result}, nil
	case "union":
		items, ok := valueItems(data, 1)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		entries, ok := valueMap(items[0])
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		variants := make(map[string]any, len(entries))
		for _, entry := range entries {
			tag, ok := entry.Key.Uint64()
			if !ok || tag > 255 {
				return nil, ErrInvalidTypeSystem
			}
			info := entry.Value.Unwrap()
			nameValue, ok := info.Field("name")
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			name, ok := nameValue.TextValue()
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			tyValue, ok := info.Field("ty")
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			semID, ok := semanticID(tyValue)
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			variants[fmt.Sprintf("%s:%d", name, tag)] = namedRef(semID)
		}
		return map[string]any{"Union": variants}, nil
	case "tuple":
		items, ok := valueItems(data, 1)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		fields, ok := valueSequence(items[0])
		if !ok || len(fields) == 0 {
			return nil, ErrInvalidTypeSystem
		}
		refs := make([]any, 0, len(fields))
		for _, field := range fields {
			semID, ok := semanticID(field)
			if !ok {
				return nil, ErrInvalidTypeSystem
			}
			refs = append(refs, namedRef(semID))
		}
		return map[string]any{"Tuple": refs}, nil
	case "struct":
		items, ok := valueItems(data, 1)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		fields, ok := valueSequence(items[0])
		if !ok || len(fields) == 0 {
			return nil, ErrInvalidTypeSystem
		}
		result := make([]any, 0, len(fields))
		for _, field := range fields {
			field = field.Unwrap()
			nameValue, nameOK := field.Field("name")
			tyValue, tyOK := field.Field("ty")
			name, textOK := nameValue.TextValue()
			semID, semOK := semanticID(tyValue)
			if !nameOK || !tyOK || !textOK || !semOK {
				return nil, ErrInvalidTypeSystem
			}
			result = append(result, map[string]any{"name": name, "ty": namedRef(semID)})
		}
		return map[string]any{"Struct": result}, nil
	case "array":
		items, ok := valueItems(data, 2)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		semID, semOK := semanticID(items[0])
		length, lenOK := items[1].Uint64()
		if !semOK || !lenOK {
			return nil, ErrInvalidTypeSystem
		}
		return map[string]any{"Array": []any{namedRef(semID), length}}, nil
	case "list", "set":
		items, ok := valueItems(data, 2)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		semID, semOK := semanticID(items[0])
		size, sizeOK := typeSizing(items[1])
		if !semOK || !sizeOK {
			return nil, ErrInvalidTypeSystem
		}
		kind := "List"
		if value.Name == "set" {
			kind = "Set"
		}
		return map[string]any{kind: []any{namedRef(semID), size}}, nil
	case "map":
		items, ok := valueItems(data, 3)
		if !ok {
			return nil, ErrInvalidTypeSystem
		}
		key, keyOK := semanticID(items[0])
		item, itemOK := semanticID(items[1])
		size, sizeOK := typeSizing(items[2])
		if !keyOK || !itemOK || !sizeOK {
			return nil, ErrInvalidTypeSystem
		}
		return map[string]any{"Map": []any{namedRef(key), namedRef(item), size}}, nil
	default:
		return nil, fmt.Errorf("unknown type system variant %q", value.Name)
	}
}

func namedRef(semID string) map[string]string { return map[string]string{"named": semID} }

func semanticID(value Value) (string, bool) {
	bytes, ok := value.Bytes()
	if !ok || len(bytes) != 32 {
		return "", false
	}
	return hex.EncodeToString(bytes), true
}

func valueItems(value Value, count int) ([]Value, bool) {
	if value.Kind != ValueTuple || len(value.Items) != count {
		return nil, false
	}
	return value.Items, true
}

func valueSequence(value Value) ([]Value, bool) {
	value = value.Unwrap()
	if value.Kind != ValueList && value.Kind != ValueSet {
		return nil, false
	}
	return value.Items, true
}

func valueMap(value Value) ([]Entry, bool) {
	value = value.Unwrap()
	if value.Kind != ValueMap {
		return nil, false
	}
	return value.Entries, true
}

func namedTag(value Value) (string, uint64, bool) {
	value = value.Unwrap()
	nameValue, nameOK := value.Field("name")
	tagValue, tagOK := value.Field("tag")
	name, textOK := nameValue.TextValue()
	tag, uintOK := tagValue.Uint64()
	return name, tag, nameOK && tagOK && textOK && uintOK && tag <= 255
}

func typeSizing(value Value) (map[string]uint64, bool) {
	value = value.Unwrap()
	minValue, minOK := value.Field("min")
	maxValue, maxOK := value.Field("max")
	min, minUint := minValue.Uint64()
	max, maxUint := maxValue.Uint64()
	if !minOK || !maxOK || !minUint || !maxUint || min > max {
		return nil, false
	}
	return map[string]uint64{"min": min, "max": max}, true
}
