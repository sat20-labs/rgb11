package browser

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestAllowlistedEsploraWitnessSnapshotIsSecondaryOnly(t *testing.T) {
	raw, txid := testWitnessTx(t)
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
	raw, txid := testWitnessTx(t)
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

func testWitnessTx(t *testing.T) ([]byte, string) {
	t.Helper()
	prevHash := chainhash.Hash{1, 2, 3, 4}
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevHash, Index: 1},
		Witness:          wire.TxWitness{[]byte{1, 2, 3}, []byte{4, 5}},
	})
	tx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: []byte{0x51}})
	var encoded bytes.Buffer
	if err := tx.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	if tx.TxHash() == tx.WitnessHash() {
		t.Fatal("test transaction must distinguish txid from wtxid")
	}
	return encoded.Bytes(), tx.TxHash().String()
}
