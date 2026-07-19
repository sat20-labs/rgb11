package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/invoicing"
)

type output struct {
	ID            string `json:"id"`
	ContractID    string `json:"contract_id"`
	SchemaID      string `json:"schema_id"`
	PayloadSHA256 string `json:"payload_sha256"`
	Invoice       string `json:"invoice"`
	SAT20Invoice  string `json:"sat20_invoice"`
}

func main() {
	contractPath := flag.String("contract", "testvectors/rc11/nia-example.rgba", "base contract fixture")
	transferPath := flag.String("transfer", "testvectors/rc11/nia-transfer.rgba", "official transfer fixture providing the witness bundle")
	outputPath := flag.String("output", "", "Go-built transfer output path")
	flag.Parse()
	if *outputPath == "" {
		fail(fmt.Errorf("output path is required"))
	}

	contractRaw, err := os.ReadFile(*contractPath)
	fail(err)
	base, err := consignment.DecodeArmor(string(contractRaw))
	fail(err)
	transferRaw, err := os.ReadFile(*transferPath)
	fail(err)
	official, err := consignment.DecodeArmor(string(transferRaw))
	fail(err)
	bundles, ok := official.Value.Field("bundles")
	if !ok || len(bundles.Unwrap().Items) == 0 {
		fail(fmt.Errorf("official transfer has no witness bundle"))
	}
	terminals, ok := official.Value.Field("terminals")
	if !ok {
		fail(fmt.Errorf("official transfer has no terminals"))
	}
	var secrets [][32]byte
	for _, terminal := range terminals.Unwrap().Entries {
		for _, item := range terminal.Value.Unwrap().Items {
			raw, ok := item.Bytes()
			if !ok || len(raw) != 32 {
				fail(fmt.Errorf("invalid terminal secret"))
			}
			var secret [32]byte
			copy(secret[:], raw)
			secrets = append(secrets, secret)
		}
	}
	built, err := consignment.BuildTransfer(base, bundles.Unwrap().Items[0], secrets)
	fail(err)
	armored, err := consignment.EncodeArmor(built.Value)
	fail(err)
	fail(os.WriteFile(*outputPath, []byte(armored), 0o600))
	parsed, err := consignment.DecodeArmor(armored)
	fail(err)
	payloadHash := sha256.Sum256(parsed.Armor.Data)

	const invoiceText = "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts/XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M/BF/bc:utxob:4vm1CX2Z-K8hMo59-e7dgGBS-Jka7mYn-Xe~yP85-yUiHHxr-aVlYa"
	invoice, err := invoicing.Parse(invoiceText)
	fail(err)
	sat20Invoice := *invoice
	sat20Invoice.UnknownQuery = []invoicing.QueryParam{
		{Key: "sat20_recipient", Value: "02d6e24c0bb9db2e5bc6ddf95be427ac363d7364a8b09c67d8540f986a1c9e1350"},
		{Key: "sat20_vout", Value: "1"},
		{Key: "sat20_relay", Value: "/tmp/1111111111111111111111111111111111111111111111111111111111111111"},
		{Key: "sat20_ack", Value: "/tmp/2222222222222222222222222222222222222222222222222222222222222222"},
	}
	result := output{
		ID: parsed.Armor.ID, ContractID: parsed.ContractID, SchemaID: parsed.SchemaID,
		PayloadSHA256: hex.EncodeToString(payloadHash[:]), Invoice: invoice.String(), SAT20Invoice: sat20Invoice.String(),
	}
	encoded, err := json.Marshal(result)
	fail(err)
	fmt.Println(string(encoded))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
