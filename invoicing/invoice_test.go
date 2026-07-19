package invoicing

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestWitnessBeneficiaryScriptRoundTrips(t *testing.T) {
	hash20 := bytes.Repeat([]byte{0x11}, 20)
	hash32 := bytes.Repeat([]byte{0xff}, 32) // Deliberately not required to be a curve point for P2WSH.
	validXOnly, err := hex.DecodeString("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	if err != nil {
		t.Fatal(err)
	}
	scripts := [][]byte{
		append(append([]byte{0x76, 0xa9, 0x14}, hash20...), 0x88, 0xac),
		append(append([]byte{0xa9, 0x14}, hash20...), 0x87),
		append([]byte{0x00, 0x14}, hash20...),
		append([]byte{0x00, 0x20}, hash32...),
		append([]byte{0x51, 0x20}, validXOnly...),
	}
	for _, script := range scripts {
		beneficiary, err := NewWitnessBeneficiary(BitcoinRegtest, script, nil)
		if err != nil {
			t.Fatalf("encode %x: %v", script, err)
		}
		parsed, err := ParseBeneficiary(beneficiary.String())
		if err != nil {
			t.Fatalf("parse %x: %v", script, err)
		}
		got, err := parsed.WitnessScript()
		if err != nil || !bytes.Equal(got, script) {
			t.Fatalf("script roundtrip got %x, %v; want %x", got, err, script)
		}
	}
}

const (
	niaInvoice     = "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts/XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M/BF/bc:utxob:4vm1CX2Z-K8hMo59-e7dgGBS-Jka7mYn-Xe~yP85-yUiHHxr-aVlYa"
	udaInvoice     = "rgb:tx8NOyGe-NkPZex~-U0J_1om-CfrOeoO-7di9xZb-vT3nxyo/XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M/1@0/bc:utxob:4vm1CX2Z-K8hMo59-e7dgGBS-Jka7mYn-Xe~yP85-yUiHHxr-aVlYa"
	witnessInvoice = "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts/XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M/Sa/bc:wvout:A8cJ7Ww3-NIzADo3-Tzp_5aD-7CTBWmA-AAAAAAA-AAAAAAA-ALSQkcw+750f58bcca0fdb11891e7979d829b8c56e0963dba08c44f54a256cf7dbc09caf"
)

func TestOfficialInvoiceRoundTrips(t *testing.T) {
	for _, vector := range []string{niaInvoice, udaInvoice, witnessInvoice,
		"rgb:~/~/~/bc:utxob:4vm1CX2Z-K8hMo59-e7dgGBS-Jka7mYn-Xe~yP85-yUiHHxr-aVlYa",
	} {
		invoice, err := Parse(vector)
		if err != nil {
			t.Fatalf("parse %q: %v", vector, err)
		}
		if got := invoice.String(); got != vector {
			t.Fatalf("roundtrip\n got %s\nwant %s", got, vector)
		}
	}
}

func TestRustDifferentialInvoiceVector(t *testing.T) {
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
	vector := corpus.Vectors["invoice_nia"]
	invoice, err := Parse(vector)
	if err != nil {
		t.Fatal(err)
	}
	if got := invoice.String(); got != vector {
		t.Fatalf("Go/Rust invoice mismatch\n got %s\nwant %s", got, vector)
	}
}

func TestOfficialAmountEncoding(t *testing.T) {
	amount, err := ParseAmount("BF")
	if err != nil || amount != 100 {
		t.Fatalf("amount = %d, %v", amount, err)
	}
	if got := Amount(100).String(); got != "BF" {
		t.Fatalf("amount encoding = %s", got)
	}
}

func TestInvoiceQueryCanonicalOrderAndEscaping(t *testing.T) {
	value := niaInvoice + "?unknown=new&expiry=1682086371&:@-%20%23=?/.%26%3D&endpoints=rpcs://host1.example.com,http://host2.example.com"
	invoice, err := Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	want := niaInvoice + "?expiry=1682086371&endpoints=rpcs://host1.example.com,http://host2.example.com&unknown=new&:@-%20%23=?/.%26%3D"
	if got := invoice.String(); got != want {
		t.Fatalf("query canonicalization\n got %s\nwant %s", got, want)
	}
}

func TestInvoiceRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"bad:" + niaInvoice[4:],
		niaInvoice + "?expiry=six",
		niaInvoice + "?assignment_name=Bad",
		niaInvoice + "?endpoints=rpc://",
	} {
		if _, err := Parse(value); err == nil {
			t.Fatalf("accepted invalid invoice %q", value)
		}
	}
}
