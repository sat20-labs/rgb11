package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/sat20-labs/rgb11/browser"
)

func main() {
	manifestPath := flag.String("manifest", "UPSTREAM_MANIFEST.json", "local release manifest")
	endpointID := flag.String("endpoint", "bitlight-regtest-esplora", "allowlisted endpoint id")
	txid := flag.String("txid", "", "public witness transaction id")
	expectedHex := flag.String("expected-hex", "", "optional primary-evidence transaction hex")
	output := flag.String("output", "", "optional response snapshot path")
	flag.Parse()

	rawManifest, err := os.ReadFile(*manifestPath)
	fail(err)
	manifest, err := browser.ParseReleaseManifest(rawManifest)
	fail(err)
	endpoint, err := manifest.Endpoint(*endpointID)
	fail(err)
	client, err := browser.NewEsploraClient(endpoint, nil)
	fail(err)
	var expected []byte
	if *expectedHex != "" {
		expected, err = hex.DecodeString(*expectedHex)
		fail(err)
	}
	report, err := client.CheckWitness(context.Background(), *txid, expected)
	fail(err)
	snapshot, err := report.SnapshotJSON()
	fail(err)
	if *output != "" {
		fail(os.WriteFile(*output, snapshot, 0o600))
	}
	fmt.Println(string(snapshot))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
