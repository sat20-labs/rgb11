package consignment

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDecodeOfficialNIAContract(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := DecodeArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !container.StructuralValid || !container.GenesisValid || container.ConsensusValid {
		t.Fatalf("unexpected validation flags: %+v", container)
	}
	if container.ContractID != "rgb:k0vsa6zj-CLYfnru-63unuJv-qZ2IVJ5-zlENzlF-MkiJNuw" {
		t.Fatalf("unexpected contract id %s", container.ContractID)
	}
}

func TestDecodeOfficialStrictBinary(t *testing.T) {
	armored, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := DecodeArmor(string(armored))
	if err != nil {
		t.Fatal(err)
	}
	binary, err := Decode(parsed.Armor.Data)
	if err != nil {
		t.Fatal(err)
	}
	if binary.ContractID != parsed.ContractID || binary.SchemaID != parsed.SchemaID || binary.Armor.Type != "transfer" {
		t.Fatalf("binary decode mismatch: %+v", binary)
	}
	file := append([]byte("RGB\x00TFR"), parsed.Armor.Data...)
	wrapped, err := Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.ContractID != parsed.ContractID || wrapped.Armor.Type != "transfer" {
		t.Fatalf("official file decode mismatch: %+v", wrapped)
	}
}

func TestStandardFileRoundTrip(t *testing.T) {
	for _, test := range []struct {
		fixture string
		kind    string
		magic   string
	}{
		{fixture: "nia-example.rgba", kind: "contract", magic: "RGB\x00CON"},
		{fixture: "nia-transfer.rgba", kind: "transfer", magic: "RGB\x00TFR"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			raw, err := os.ReadFile("../testvectors/rc11/" + test.fixture)
			if err != nil {
				t.Fatal(err)
			}
			container, err := DecodeArmor(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			file, err := EncodeFile(container)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(file), test.magic) {
				t.Fatalf("file magic=%q want=%q", file[:7], test.magic)
			}
			decoded, err := DecodeFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.ContractID != container.ContractID || decoded.SchemaID != container.SchemaID ||
				decoded.Armor.Type != test.kind {
				t.Fatalf("standard file round trip mismatch: %+v", decoded)
			}
		})
	}
}

func TestDecodeFileRejectsNonStandardInputs(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := DecodeArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string][]byte{
		"armor":  raw,
		"strict": container.Armor.Data,
		"magic":  append([]byte("RGB\x00BAD"), container.Armor.Data...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeFile(input); !errors.Is(err, ErrFileMagic) {
				t.Fatalf("error=%v want=%v", err, ErrFileMagic)
			}
		})
	}

	wrongType := append([]byte("RGB\x00CON"), container.Armor.Data...)
	if _, err := DecodeFile(wrongType); err == nil {
		t.Fatal("accepted transfer payload with contract file magic")
	}
}

func TestDecodeEveryOfficialWalletSchemaContract(t *testing.T) {
	for _, fixture := range []string{
		"nia-example.rgba",
		"ifa-example.rgba",
		"cfa-example.rgba",
		"uda-example.rgba",
	} {
		t.Run(fixture, func(t *testing.T) {
			raw, err := os.ReadFile("../testvectors/rc11/" + fixture)
			if err != nil {
				t.Fatal(err)
			}
			container, err := DecodeArmor(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			if container.Armor.Type != "contract" || !container.StructuralValid || !container.GenesisValid || container.ConsensusValid {
				t.Fatalf("unexpected validation flags: %+v", container)
			}
		})
	}
}

func TestDecodeGeneratedOfficialNIATransfer(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-transfer.rgba")
	if err != nil {
		t.Fatal(err)
	}
	container, err := DecodeArmor(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if container.Armor.Type != "transfer" || !container.StructuralValid || !container.GenesisValid || container.ConsensusValid {
		t.Fatalf("unexpected transfer validation flags: %+v", container)
	}
	if container.ContractID != "rgb:k0vsa6zj-CLYfnru-63unuJv-qZ2IVJ5-zlENzlF-MkiJNuw" {
		t.Fatalf("unexpected contract id %s", container.ContractID)
	}
}

func TestDecodeRejectsForgedContractHeader(t *testing.T) {
	raw, err := os.ReadFile("../testvectors/rc11/nia-example.rgba")
	if err != nil {
		t.Fatal(err)
	}
	forged := strings.Replace(string(raw),
		"Contract: rgb:k0vsa6zj-CLYfnru-63unuJv-qZ2IVJ5-zlENzlF-MkiJNuw",
		"Contract: rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E", 1)
	if _, err := DecodeArmor(forged); !errors.Is(err, ErrContractMismatch) {
		t.Fatalf("expected contract mismatch, got %v", err)
	}
}
