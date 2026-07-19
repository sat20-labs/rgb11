package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowlistedEsploraWitnessSnapshotIsSecondaryOnly(t *testing.T) {
	raw := []byte{1, 2, 3, 4, 5}
	txid := testTxID(raw)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tx/" + txid + "/status":
			fmt.Fprint(w, `{"confirmed":true,"block_height":101,"block_hash":"abc"}`)
		case "/tx/" + txid + "/hex":
			fmt.Fprint(w, hex.EncodeToString(raw))
		case "/tx/" + txid + "/outspends":
			fmt.Fprint(w, `[{"spent":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	endpoint := Endpoint{
		ID: "bitlight-regtest-esplora", Kind: "esplora", BaseURL: server.URL,
		Network: "regtest", RecognitionURL: "https://rgb.tech/integrate/",
	}
	client, err := NewEsploraClient(endpoint, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	report, err := client.CheckWitness(context.Background(), txid, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Available || !report.MatchesExpectedRaw || report.ObservedTxID != txid || report.ConsensusAuthority || len(report.Differences) != 0 {
		t.Fatalf("unexpected secondary report: %+v", report)
	}
	if len(report.Responses) != 3 || report.Responses[1].BodySHA256 == "" {
		t.Fatalf("browser response snapshots missing: %+v", report.Responses)
	}
	if snapshot, err := report.SnapshotJSON(); err != nil || len(snapshot) == 0 {
		t.Fatalf("snapshot JSON: %v", err)
	}
}

func TestEsploraDifferenceDoesNotBecomeConsensusResult(t *testing.T) {
	raw := []byte{9, 8, 7}
	txid := testTxID(raw)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tx/"+txid+"/hex" {
			fmt.Fprint(w, hex.EncodeToString(raw))
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client, err := NewEsploraClient(Endpoint{
		ID: "test", Kind: "esplora", BaseURL: server.URL, Network: "regtest",
		RecognitionURL: "https://rgb.tech/integrate/",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	report, err := client.CheckWitness(context.Background(), txid, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if report.MatchesExpectedRaw || report.ConsensusAuthority || len(report.Differences) == 0 {
		t.Fatalf("expected diagnostic-only mismatch: %+v", report)
	}
}

func TestReleaseManifestRejectsNonAllowlistedHTTP(t *testing.T) {
	raw := []byte(`{"secondary_oracles":[{"id":"remote","kind":"esplora","base_url":"http://example.com","network":"mainnet","recognition_url":"https://rgb.tech/integrate/"}]}`)
	if _, err := ParseReleaseManifest(raw); err == nil {
		t.Fatal("accepted non-TLS remote browser endpoint")
	}
}

func testTxID(raw []byte) string {
	first := sha256.Sum256(raw)
	second := sha256.Sum256(first[:])
	for left, right := 0, len(second)-1; left < right; left, right = left+1, right-1 {
		second[left], second[right] = second[right], second[left]
	}
	return hex.EncodeToString(second[:])
}
