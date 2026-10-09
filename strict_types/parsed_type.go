package strict_types

import (
	"encoding/json"
	"fmt"
	"sort"
)

// parsedType is built when a library is installed. Binary operations only read
// this tree; references remain identifiers so forward references and separate
// contract type systems keep their existing resolution rules.
type parsedType struct {
	raw       json.RawMessage
	kind      string
	primitive uint8
	variants  []parsedVariant
	refs      []typeRef
	fields    []parsedField
	length    uint16
	limits    sizing
}

type parsedVariant struct {
	name string
	tag  uint8
	ref  typeRef
}

type parsedField struct {
	Name string  `json:"name"`
	Type typeRef `json:"ty"`
}

func parseType(raw json.RawMessage) (*parsedType, error) {
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(raw, &variants); err != nil || len(variants) != 1 {
		return nil, ErrMalformedType
	}
	result := &parsedType{raw: append(json.RawMessage(nil), raw...)}
	for kind, body := range variants {
		result.kind = kind
		switch kind {
		case "Primitive":
			if json.Unmarshal(body, &result.primitive) != nil {
				return nil, ErrMalformedType
			}
		case "UnicodeChar", "Unicode":
		case "Enum":
			var names []string
			if json.Unmarshal(body, &names) != nil {
				return nil, ErrMalformedType
			}
			for _, value := range names {
				name, tag, err := parseVariant(value)
				if err != nil {
					return nil, err
				}
				result.variants = append(result.variants, parsedVariant{name: name, tag: tag})
			}
		case "Union":
			var refs map[string]typeRef
			if json.Unmarshal(body, &refs) != nil {
				return nil, ErrMalformedType
			}
			// Stable iteration also gives deterministic errors for malformed values.
			names := make([]string, 0, len(refs))
			for name := range refs {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, value := range names {
				name, tag, err := parseVariant(value)
				if err != nil {
					return nil, err
				}
				result.variants = append(result.variants, parsedVariant{name: name, tag: tag, ref: refs[value]})
			}
		case "Tuple":
			if json.Unmarshal(body, &result.refs) != nil {
				return nil, ErrMalformedType
			}
		case "Struct":
			if json.Unmarshal(body, &result.fields) != nil {
				return nil, ErrMalformedType
			}
		case "Array", "List", "Set", "Map":
			var parts []json.RawMessage
			count := 2
			if kind == "Map" {
				count = 3
			}
			if json.Unmarshal(body, &parts) != nil || len(parts) != count {
				return nil, ErrMalformedType
			}
			for _, part := range parts[:count-1] {
				var ref typeRef
				if json.Unmarshal(part, &ref) != nil {
					return nil, ErrMalformedType
				}
				result.refs = append(result.refs, ref)
			}
			if kind == "Array" {
				if json.Unmarshal(parts[1], &result.length) != nil {
					return nil, ErrMalformedType
				}
			} else if json.Unmarshal(parts[count-1], &result.limits) != nil || result.limits.Min > result.limits.Max {
				return nil, ErrMalformedType
			}
		default:
			return nil, fmt.Errorf("%w %q", ErrMalformedType, kind)
		}
	}
	return result, nil
}

// UnmarshalJSON is only used at installation, including nested inline types.
func (r *typeRef) UnmarshalJSON(raw []byte) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != 1 {
		return ErrMalformedType
	}
	*r = typeRef{}
	if value, ok := object["named"]; ok {
		var semID string
		if json.Unmarshal(value, &semID) != nil || len(semID) != 64 {
			return ErrMalformedType
		}
		r.Named = &semID
		return nil
	}
	if value, ok := object["extern"]; ok {
		var external struct {
			LibID string `json:"libId"`
			SemID string `json:"semId"`
		}
		if json.Unmarshal(value, &external) != nil || len(external.LibID) != 64 || len(external.SemID) != 64 {
			return ErrMalformedType
		}
		r.Extern = &external
		return nil
	}
	if value, ok := object["inline"]; ok {
		parsed, err := parseType(value)
		if err != nil {
			return err
		}
		r.Inline = parsed
		return nil
	}
	return ErrMalformedType
}
