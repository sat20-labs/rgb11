// Package strict_types contains the RGB strict-types 1.0.2 primitives needed
// by the translated consensus and invoicing layers.
package strict_types

import "errors"

// Upstream-Repository: rgb-protocol/rgb-strict-encoding
// Upstream-Version: 1.0.2
// Upstream-Commit: 7698a5e96a2a27d5bfa4cd3560da0e8af8e4a18a
// Upstream-File: rust/src/ident.rs
// Translation-Revision: 1

const IdentMaxLen = 100

var ErrInvalidIdentifier = errors.New("invalid RGB11 strict identifier")

type IdentifierKind uint8

const (
	IdentAny IdentifierKind = iota
	TypeName
	FieldName
	VariantName
	LibraryName
)

type Identifier struct {
	Kind  IdentifierKind
	Value string
}

func NewIdentifier(kind IdentifierKind, value string) (Identifier, error) {
	if !validIdentifier(kind, value) {
		return Identifier{}, ErrInvalidIdentifier
	}
	return Identifier{Kind: kind, Value: value}, nil
}

func (i Identifier) String() string { return i.Value }

func ValidFieldName(value string) bool { return validIdentifier(FieldName, value) }

func validIdentifier(kind IdentifierKind, value string) bool {
	if len(value) == 0 || len(value) > IdentMaxLen {
		return false
	}
	first := value[0]
	switch kind {
	case TypeName, LibraryName:
		if !((first >= 'A' && first <= 'Z') || first == '_') {
			return false
		}
	case FieldName, VariantName:
		if !((first >= 'a' && first <= 'z') || first == '_') {
			return false
		}
	default:
		if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
			return false
		}
	}
	for _, char := range []byte(value[1:]) {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_') {
			return false
		}
	}
	return true
}
