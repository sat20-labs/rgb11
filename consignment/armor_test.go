package consignment

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestParseOfficialTransferArmor(t *testing.T) {
	path := "../testvectors/rc11/armored_transfer.txt"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	armor, err := ParseArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if armor.Type != "transfer" || armor.Version != 0 || len(armor.Data) == 0 {
		t.Fatalf("unexpected armor %+v", armor)
	}
}

func TestArmorChecksumFailClosed(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/armored_transfer.txt")
	if err != nil {
		t.Fatal(err)
	}
	offset := bytes.Index(raw, []byte("009610"))
	if offset < 0 {
		t.Fatal("test body missing")
	}
	raw[offset] = '1'
	if _, err := ParseArmor(string(raw)); !errors.Is(err, ErrArmorChecksum) {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestBase85Vectors(t *testing.T) {
	tests := map[string]string{"VE": "a", "VPO": "aa", "VPRn": "aaa", "VPRom": "aaaa"}
	for encoded, want := range tests {
		got, err := decodeBase85(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("decode %s = %q, want %q", encoded, got, want)
		}
		if got := encodeBase85([]byte(want)); got != encoded {
			t.Fatalf("encode %q = %q, want %q", want, got, encoded)
		}
	}
	if _, err := decodeBase85("x"); !errors.Is(err, ErrBase85Remainder) {
		t.Fatalf("expected remainder error, got %v", err)
	}
}

func TestEncodeOfficialConsignmentRoundTrip(t *testing.T) {
	for _, fixture := range []struct {
		path     string
		typeName string
	}{
		{"../testvectors/rc11/nia-example.rgba", "Consignmentfalse"},
		{"../testvectors/rc11/nia-transfer.rgba", "Consignmenttrue"},
	} {
		value := decodeOfficialValue(t, fixture.path, fixture.typeName)
		armored, err := EncodeArmor(value)
		if err != nil {
			t.Fatalf("%s: %v", fixture.path, err)
		}
		decoded, err := DecodeArmor(armored)
		if err != nil {
			t.Fatalf("round-trip %s: %v", fixture.path, err)
		}
		if !bytes.Equal(decoded.Armor.Data, value.Encoded) {
			t.Fatalf("round-trip %s changed strict payload", fixture.path)
		}
	}
}
