package strict_types

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

func TestRC11RegistrySharesFixedTypesButKeepsRegistrationIndependent(t *testing.T) {
	first, err := RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	second, err := RC11Registry()
	if err != nil {
		t.Fatal(err)
	}
	if first.byName["RGBStd"] != second.byName["RGBStd"] {
		t.Fatal("fixed RC11 type definitions are parsed again for every registry")
	}
	extra := []byte(`{"id":"` + strings.Repeat("a", 64) + `","namedSemIds":{"Byte":"` + strings.Repeat("b", 64) + `"},"library":{"name":"LocalOnly","types":{"Byte":{"Primitive":64}}}}`)
	if err := first.AddLibrary(extra); err != nil {
		t.Fatal(err)
	}
	if _, err := second.library("LocalOnly"); !errors.Is(err, ErrLibraryAbsent) {
		t.Fatalf("registration leaked to another contract: %v", err)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 16; j++ {
				value, err := first.Decode("LocalOnly", "Byte", []byte{byte(j)})
				if err != nil {
					t.Error(err)
					return
				}
				encoded, err := first.Encode("LocalOnly", "Byte", value)
				if err != nil || len(encoded) != 1 || encoded[0] != byte(j) {
					t.Errorf("concurrent round trip: %x %v", encoded, err)
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestParsedDefinitionsRetainStrictValueValidation(t *testing.T) {
	registry := NewRegistry()
	library := []byte(`{"id":"` + strings.Repeat("a", 64) + `","library":{"name":"Validation","types":{
		"Byte":{"Primitive":64},
		"Letters":{"List":[{"inline":{"Enum":["a:97","b:98"]}},{"min":1,"max":4}]},
		"Text":{"List":[{"inline":{"UnicodeChar":null}},{"min":1,"max":4}]},
		"Set":{"Set":[{"inline":{"Primitive":64}},{"min":0,"max":4}]},
		"Map":{"Map":[{"inline":{"Primitive":64}},{"inline":{"Primitive":64}},{"min":0,"max":4}]}
	}}}`)
	if err := registry.AddLibrary(library); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, typeName string
		data           []byte
		want           error
	}{
		{"trailing data", "Byte", []byte{1, 2}, strict.ErrTrailingData},
		{"unknown character", "Letters", []byte{1, 'c'}, ErrUnknownVariant},
		{"minimum length", "Letters", []byte{0}, strict.ErrOutOfBounds},
		{"maximum length", "Letters", []byte{5, 'a', 'a', 'a', 'a', 'a'}, strict.ErrOutOfBounds},
		{"invalid UTF8", "Text", []byte{1, 0xff}, nil},
		{"unordered set", "Set", []byte{2, 2, 1}, ErrNonCanonical},
		{"duplicate set", "Set", []byte{2, 1, 1}, ErrNonCanonical},
		{"unordered map", "Map", []byte{2, 2, 0, 1, 0}, ErrNonCanonical},
		{"duplicate map", "Map", []byte{2, 1, 0, 1, 0}, ErrNonCanonical},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.Decode("Validation", test.typeName, test.data)
			if err == nil || (test.want != nil && !errors.Is(err, test.want)) {
				t.Fatalf("invalid value accepted or wrong error: %v; want %v", err, test.want)
			}
		})
	}
	for _, test := range []struct {
		typeName string
		data     []byte
	}{
		{"Letters", []byte{2, 'a', 'b'}},
		{"Text", []byte{2, 0xc3, 0xa9}},
		{"Set", []byte{2, 1, 2}},
		{"Map", []byte{2, 1, 9, 2, 8}},
	} {
		value, err := registry.Decode("Validation", test.typeName, test.data)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := registry.Encode("Validation", test.typeName, value)
		if err != nil || !bytes.Equal(encoded, test.data) {
			t.Fatalf("%s canonical round trip: %x %v", test.typeName, encoded, err)
		}
	}
}

func TestRegistryRejectsMalformedDefinitionBeforePublishingLibrary(t *testing.T) {
	registry := NewRegistry()
	library := []byte(`{"id":"` + strings.Repeat("a", 64) + `","namedSemIds":{"Broken":"` + strings.Repeat("b", 64) + `"},"library":{"name":"BrokenLib","types":{"Broken":{"Primitive":999}}}}`)
	if err := registry.AddLibrary(library); !errors.Is(err, ErrMalformedType) {
		t.Fatalf("malformed type accepted at load: %v", err)
	}
	if _, err := registry.library("BrokenLib"); !errors.Is(err, ErrLibraryAbsent) {
		t.Fatalf("failed load published a partial library: %v", err)
	}
}
