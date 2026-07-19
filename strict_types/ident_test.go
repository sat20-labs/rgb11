package strict_types

import "testing"

func TestStrictIdentifierClasses(t *testing.T) {
	valid := []struct {
		kind  IdentifierKind
		value string
	}{{FieldName, "assetOwner"}, {TypeName, "NonInflatableAsset"}, {LibraryName, "RGBContract"}, {IdentAny, "Some_name2"}}
	for _, test := range valid {
		if _, err := NewIdentifier(test.kind, test.value); err != nil {
			t.Fatalf("rejected %q: %v", test.value, err)
		}
	}
	for _, value := range []string{"", "Bad", "0owner", "owner-name"} {
		if ValidFieldName(value) {
			t.Fatalf("accepted invalid field name %q", value)
		}
	}
}
