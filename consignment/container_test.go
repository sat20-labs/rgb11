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
