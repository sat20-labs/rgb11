package schemas

import "testing"

func TestFrozenStandardSchemaCatalog(t *testing.T) {
	if len(Standard) != 5 {
		t.Fatalf("schema count = %d", len(Standard))
	}
	for _, descriptor := range Standard {
		got, err := ByID(descriptor.SchemaID)
		if err != nil {
			t.Fatalf("%s: %v", descriptor.Kind, err)
		}
		if got.Kind != descriptor.Kind || got.SourceSHA256 == "" {
			t.Fatalf("catalog mismatch: %+v", got)
		}
	}
}
