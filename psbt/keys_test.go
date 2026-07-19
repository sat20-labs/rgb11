package psbt

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestRGBTransitionRawKeyEncoding(t *testing.T) {
	var operation [32]byte
	for index := range operation {
		operation[index] = byte(index)
	}
	key := RGBTransition(operation)
	encoded, err := key.SerializeKey()
	if err != nil {
		t.Fatal(err)
	}
	want := "26fc0352474201" + hex.EncodeToString(operation[:])
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("serialized key = %s, want %s", got, want)
	}
	data, err := key.KeyData()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseKeyData(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed.Prefix, RGBPrefix) || parsed.Subtype != GlobalRGBTransition || !bytes.Equal(parsed.Key, operation[:]) {
		t.Fatalf("parsed key = %+v", parsed)
	}
	raw, err := os.ReadFile("../testvectors/rc11/core.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Vectors map[string]string `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(data), corpus.Vectors["psbt_rgb_transition_key_data_hex"]; got != want {
		t.Fatalf("Go/Rust proprietary key mismatch: got %s want %s", got, want)
	}
	rawKey, err := key.RawKey()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(rawKey), "fc"+hex.EncodeToString(data); got != want {
		t.Fatalf("raw key = %s, want %s", got, want)
	}
}

func TestOfficialProprietaryNamespaces(t *testing.T) {
	cases := []struct {
		key    ProprietaryKey
		prefix string
		typeID byte
	}{
		{MPCEntropy(), "MPC", OutMPCEntropy},
		{OpretHost(), "OPRET", OutOpretHost},
		{TapretProof(), "TAPRET", OutTapretProof},
		{RGBCloseMethod(), "RGB", GlobalRGBCloseMethod},
	}
	for _, test := range cases {
		if string(test.key.Prefix) != test.prefix || test.key.Subtype != test.typeID {
			t.Fatalf("wrong proprietary key %+v", test.key)
		}
	}
}

func TestTerminalRoundTrip(t *testing.T) {
	terminal, err := ParseTerminal("&1/42")
	if err != nil {
		t.Fatal(err)
	}
	if terminal.String() != "&1/42" {
		t.Fatal(terminal)
	}
}
