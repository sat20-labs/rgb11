package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrozenFullValidationCases(t *testing.T) {
	t.Chdir("../..")
	dir := t.TempDir()
	if err := writeValidationCases(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []validationCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 44 {
		t.Fatalf("full differential cases=%d want=44", len(cases))
	}
	goldenRaw, err := os.ReadFile("testvectors/rc11/validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden []struct {
		Name    string `json:"name"`
		Outcome string `json:"outcome"`
		SHA256  string `json:"payload_sha256"`
	}
	if err := json.Unmarshal(goldenRaw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) != len(cases) {
		t.Fatal("frozen differential corpus size mismatch")
	}
	for index, item := range cases {
		payload, err := hex.DecodeString(item.PayloadHex)
		if item.Binary {
			payload, err = os.ReadFile(item.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(payload)
		if golden[index].Name != item.Name || golden[index].Outcome != item.GoOutcome || golden[index].SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("frozen case changed: %s", item.Name)
		}
		if item.Expected != item.GoOutcome {
			t.Fatalf("%s: %s != %s", item.Name, item.Expected, item.GoOutcome)
		}
		// Semantic mutations must reach schema rules, never fail only at an ID
		// or a stale Bitcoin commitment. IDs and commitments are rebuilt above.
		if strings.HasSuffix(item.Name, "-invalid-state") || item.Name == "ifa-inflate-excess" {
			if item.GoStage != "schema_script" {
				t.Fatalf("%s failed before script validation: %s", item.Name, item.GoError)
			}
		}
	}
}
