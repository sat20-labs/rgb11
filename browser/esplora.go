// Package browser provides release-manifest allowlisted, secondary public-data
// checks. Browser results are snapshots for diagnostics and never participate
// in RGB consensus validation or wallet balance projection.
package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

var ErrInvalidEndpoint = errors.New("invalid RGB11 browser endpoint")

type Endpoint struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	BaseURL          string `json:"base_url"`
	Network          string `json:"network"`
	RecognitionURL   string `json:"recognition_url"`
	EnabledByDefault bool   `json:"enabled_by_default"`
}

type ReleaseManifest struct {
	SecondaryOracles []Endpoint `json:"secondary_oracles"`
}

func ParseReleaseManifest(raw []byte) (*ReleaseManifest, error) {
	var manifest ReleaseManifest
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.SecondaryOracles) == 0 {
		return nil, ErrInvalidEndpoint
	}
	seen := make(map[string]struct{}, len(manifest.SecondaryOracles))
	for _, endpoint := range manifest.SecondaryOracles {
		if _, ok := seen[endpoint.ID]; ok || validateEndpoint(endpoint) != nil {
			return nil, ErrInvalidEndpoint
		}
		seen[endpoint.ID] = struct{}{}
	}
	return &manifest, nil
}

func (m ReleaseManifest) Endpoint(id string) (Endpoint, error) {
	for _, endpoint := range m.SecondaryOracles {
		if endpoint.ID == id {
			return endpoint, nil
		}
	}
	return Endpoint{}, ErrInvalidEndpoint
}

type ResponseSnapshot struct {
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
	BodySHA256 string `json:"body_sha256"`
}

type WitnessReport struct {
	Version            uint32             `json:"version"`
	EndpointID         string             `json:"endpoint_id"`
	EndpointKind       string             `json:"endpoint_kind"`
	RecognitionURL     string             `json:"recognition_url"`
	TxID               string             `json:"txid"`
	ObservedTxID       string             `json:"observed_txid,omitempty"`
	Available          bool               `json:"available"`
	MatchesExpectedRaw bool               `json:"matches_expected_raw"`
	ConsensusAuthority bool               `json:"consensus_authority"`
	Differences        []string           `json:"differences,omitempty"`
	Responses          []ResponseSnapshot `json:"responses"`
	CapturedAt         int64              `json:"captured_at"`
}

func (r WitnessReport) SnapshotJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

type EsploraClient struct {
	endpoint Endpoint
	http     *http.Client
	base     *url.URL
}

func NewEsploraClient(endpoint Endpoint, client *http.Client) (*EsploraClient, error) {
	if err := validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	base, _ := url.Parse(endpoint.BaseURL)
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &EsploraClient{endpoint: endpoint, http: client, base: base}, nil
}

// CheckWitness snapshots the public Esplora status, raw transaction and
// outspends. A mismatch is reported as data; it never overrides the official
// Rust/Go consensus result.
func (c *EsploraClient) CheckWitness(ctx context.Context, txid string, expectedRaw []byte) (*WitnessReport, error) {
	if c == nil || c.base == nil || !validTxID(txid) {
		return nil, ErrInvalidEndpoint
	}
	report := &WitnessReport{
		Version: 1, EndpointID: c.endpoint.ID, EndpointKind: c.endpoint.Kind,
		RecognitionURL: c.endpoint.RecognitionURL, TxID: strings.ToLower(txid),
		MatchesExpectedRaw: len(expectedRaw) == 0, ConsensusAuthority: false,
		CapturedAt: time.Now().Unix(),
	}
	paths := []string{"/tx/" + strings.ToLower(txid) + "/status", "/tx/" + strings.ToLower(txid) + "/hex", "/tx/" + strings.ToLower(txid) + "/outspends"}
	var rawTx []byte
	for _, path := range paths {
		body, status, err := c.get(ctx, path)
		if err != nil {
			return report, err
		}
		hash := sha256.Sum256(body)
		report.Responses = append(report.Responses, ResponseSnapshot{
			Path: path, StatusCode: status, Body: string(body), BodySHA256: hex.EncodeToString(hash[:]),
		})
		if status < 200 || status >= 300 {
			report.Differences = append(report.Differences, fmt.Sprintf("%s returned HTTP %d", path, status))
			continue
		}
		if strings.HasSuffix(path, "/hex") {
			decoded, err := hex.DecodeString(strings.TrimSpace(string(body)))
			if err != nil || len(decoded) == 0 {
				report.Differences = append(report.Differences, "Esplora returned invalid transaction hex")
				continue
			}
			rawTx = decoded
		}
	}
	report.Available = len(report.Responses) == len(paths)
	if len(rawTx) != 0 {
		report.ObservedTxID = bitcoinTxID(rawTx)
		if report.ObservedTxID != report.TxID {
			report.Differences = append(report.Differences, "Esplora transaction bytes do not match requested txid")
		}
		if len(expectedRaw) != 0 {
			report.MatchesExpectedRaw = string(rawTx) == string(expectedRaw)
			if !report.MatchesExpectedRaw {
				report.Differences = append(report.Differences, "Esplora transaction bytes differ from primary Bitcoin evidence")
			}
		}
	}
	return report, nil
}

func (c *EsploraClient) get(ctx context.Context, path string) ([]byte, int, error) {
	target := *c.base
	target.Path = strings.TrimRight(c.base.Path, "/") + path
	target.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("%w: response too large", ErrInvalidEndpoint)
	}
	return body, resp.StatusCode, nil
}

func validateEndpoint(endpoint Endpoint) error {
	if endpoint.ID == "" || endpoint.Kind != "esplora" || endpoint.BaseURL == "" || endpoint.Network == "" || endpoint.RecognitionURL == "" {
		return ErrInvalidEndpoint
	}
	base, err := url.Parse(endpoint.BaseURL)
	if err != nil || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Hostname() == "" {
		return ErrInvalidEndpoint
	}
	if base.Scheme == "https" {
		return nil
	}
	if base.Scheme == "http" && isLoopbackHost(base.Hostname()) {
		return nil
	}
	return ErrInvalidEndpoint
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func validTxID(txid string) bool {
	decoded, err := hex.DecodeString(txid)
	return err == nil && len(decoded) == 32
}

func bitcoinTxID(raw []byte) string {
	first := sha256.Sum256(raw)
	second := sha256.Sum256(first[:])
	for left, right := 0, len(second)-1; left < right; left, right = left+1, right-1 {
		second[left], second[right] = second[right], second[left]
	}
	return hex.EncodeToString(second[:])
}
