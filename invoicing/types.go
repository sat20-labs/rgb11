package invoicing

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
)

// Upstream-Repository: rgb-protocol/rgb-ops
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 5308b9d46c91857513ff5be2459992264687632b
// Upstream-File: invoice/src/invoice.rs
// Translation-Revision: 1

type ChainNet string

const (
	BitcoinMainnet      ChainNet = "bc"
	BitcoinTestnet3     ChainNet = "tb3"
	BitcoinTestnet4     ChainNet = "tb4"
	BitcoinSignet       ChainNet = "sb"
	BitcoinSignetCustom ChainNet = "sbc"
	BitcoinRegtest      ChainNet = "bcrt"
	LiquidMainnet       ChainNet = "lq"
	LiquidTestnet       ChainNet = "tl"
)

func ParseChainNet(value string) (ChainNet, error) {
	network := ChainNet(value)
	switch network {
	case BitcoinMainnet, BitcoinTestnet3, BitcoinTestnet4, BitcoinSignet,
		BitcoinSignetCustom, BitcoinRegtest, LiquidMainnet, LiquidTestnet:
		return network, nil
	default:
		return "", fmt.Errorf("%w: network %q", ErrInvalidBeneficiary, value)
	}
}

type TransportKind string

const (
	TransportJSONRPC    TransportKind = "rpc"
	TransportRESTHTTP   TransportKind = "http"
	TransportWebSockets TransportKind = "ws"
	TransportStorm      TransportKind = "storm"
)

type Transport struct {
	Kind TransportKind `json:"kind"`
	TLS  bool          `json:"tls"`
	Host string        `json:"host"`
}

func ParseTransport(value string) (Transport, error) {
	kind, host, ok := strings.Cut(value, "://")
	if !ok || host == "" {
		return Transport{}, fmt.Errorf("%w: %q", ErrInvalidTransport, value)
	}
	switch kind {
	case "rpc":
		return Transport{Kind: TransportJSONRPC, Host: host}, nil
	case "rpcs":
		return Transport{Kind: TransportJSONRPC, TLS: true, Host: host}, nil
	case "http":
		return Transport{Kind: TransportRESTHTTP, Host: host}, nil
	case "https":
		return Transport{Kind: TransportRESTHTTP, TLS: true, Host: host}, nil
	case "ws":
		return Transport{Kind: TransportWebSockets, Host: host}, nil
	case "wss":
		return Transport{Kind: TransportWebSockets, TLS: true, Host: host}, nil
	case "storm":
		return Transport{Kind: TransportStorm, Host: host}, nil
	default:
		return Transport{}, fmt.Errorf("%w: %q", ErrInvalidTransport, value)
	}
}

func (t Transport) String() string {
	switch t.Kind {
	case TransportJSONRPC:
		if t.TLS {
			return "rpcs://" + t.Host
		}
		return "rpc://" + t.Host
	case TransportRESTHTTP:
		if t.TLS {
			return "https://" + t.Host
		}
		return "http://" + t.Host
	case TransportWebSockets:
		if t.TLS {
			return "wss://" + t.Host
		}
		return "ws://" + t.Host
	case TransportStorm:
		return "storm://_/"
	default:
		return ""
	}
}

type StateKind uint8

const (
	StateVoid StateKind = iota
	StateAmount
	StateAllocation
)

type InvoiceState struct {
	Kind       StateKind `json:"kind"`
	Amount     Amount    `json:"amount,omitempty"`
	Fraction   uint64    `json:"fraction,omitempty"`
	TokenIndex uint32    `json:"token_index,omitempty"`
}

func ParseInvoiceState(value string) (InvoiceState, error) {
	if value == "" {
		return InvoiceState{Kind: StateVoid}, nil
	}
	if amount, err := ParseAmount(value); err == nil {
		return InvoiceState{Kind: StateAmount, Amount: amount}, nil
	}
	fraction, token, ok := strings.Cut(value, "@")
	if !ok {
		return InvoiceState{}, fmt.Errorf("%w: %q", ErrInvalidState, value)
	}
	f, err := strconv.ParseUint(fraction, 10, 64)
	if err != nil {
		return InvoiceState{}, fmt.Errorf("%w: fraction %q", ErrInvalidState, fraction)
	}
	i, err := strconv.ParseUint(token, 10, 32)
	if err != nil {
		return InvoiceState{}, fmt.Errorf("%w: token index %q", ErrInvalidState, token)
	}
	return InvoiceState{Kind: StateAllocation, Fraction: f, TokenIndex: uint32(i)}, nil
}

func (s InvoiceState) String() string {
	switch s.Kind {
	case StateVoid:
		return ""
	case StateAmount:
		return s.Amount.String()
	case StateAllocation:
		return strconv.FormatUint(s.Fraction, 10) + "@" + strconv.FormatUint(uint64(s.TokenIndex), 10)
	default:
		return ""
	}
}

type BeneficiaryKind uint8

const (
	BeneficiaryBlindedSeal BeneficiaryKind = iota + 1
	BeneficiaryWitnessVout
)

type Beneficiary struct {
	Network       ChainNet             `json:"network"`
	Kind          BeneficiaryKind      `json:"kind"`
	BlindedSeal   consensus.SecretSeal `json:"blinded_seal,omitempty"`
	WitnessVout   [33]byte             `json:"witness_vout,omitempty"`
	InternalXOnly *[32]byte            `json:"internal_x_only,omitempty"`
}

func ParseBeneficiary(value string) (Beneficiary, error) {
	netText, payload, ok := strings.Cut(value, ":")
	if !ok || payload == "" {
		return Beneficiary{}, ErrInvalidBeneficiary
	}
	network, err := ParseChainNet(netText)
	if err != nil {
		return Beneficiary{}, err
	}
	if seal, err := consensus.ParseSecretSeal(payload); err == nil {
		return Beneficiary{Network: network, Kind: BeneficiaryBlindedSeal, BlindedSeal: seal}, nil
	}
	witnessText, internalText, hasInternal := strings.Cut(payload, "+")
	witness, err := baid64.Decode(witnessText, 33, baid64.WitnessVoutOptions())
	if err != nil {
		return Beneficiary{}, fmt.Errorf("%w: %v", ErrInvalidBeneficiary, err)
	}
	if witness[0] < 1 || witness[0] > 5 {
		return Beneficiary{}, fmt.Errorf("%w: witness type %d", ErrInvalidBeneficiary, witness[0])
	}
	if (witness[0] == 4 || witness[0] == 5) && !validXOnly(witness[1:]) {
		return Beneficiary{}, fmt.Errorf("%w: invalid witness key/hash", ErrInvalidBeneficiary)
	}
	var witnessArray [33]byte
	copy(witnessArray[:], witness)
	beneficiary := Beneficiary{Network: network, Kind: BeneficiaryWitnessVout, WitnessVout: witnessArray}
	if hasInternal {
		decoded, err := hex.DecodeString(internalText)
		if err != nil || !validXOnly(decoded) {
			return Beneficiary{}, fmt.Errorf("%w: invalid internal key", ErrInvalidBeneficiary)
		}
		var internal [32]byte
		copy(internal[:], decoded)
		beneficiary.InternalXOnly = &internal
	}
	return beneficiary, nil
}

func (b Beneficiary) String() string {
	prefix := string(b.Network) + ":"
	switch b.Kind {
	case BeneficiaryBlindedSeal:
		return prefix + b.BlindedSeal.String()
	case BeneficiaryWitnessVout:
		encoded, err := baid64.Encode(b.WitnessVout[:], baid64.WitnessVoutOptions())
		if err != nil {
			return ""
		}
		if b.InternalXOnly != nil {
			encoded += "+" + hex.EncodeToString(b.InternalXOnly[:])
		}
		return prefix + encoded
	default:
		return ""
	}
}

var secpP = func() *big.Int {
	p, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	return p
}()

// validXOnly checks that an x coordinate lifts to a secp256k1 point.
func validXOnly(value []byte) bool {
	if len(value) != 32 {
		return false
	}
	x := new(big.Int).SetBytes(value)
	if x.Cmp(secpP) >= 0 {
		return false
	}
	y2 := new(big.Int).Exp(x, big.NewInt(3), secpP)
	y2.Add(y2, big.NewInt(7)).Mod(y2, secpP)
	exponent := new(big.Int).Add(secpP, big.NewInt(1))
	exponent.Rsh(exponent, 2)
	y := new(big.Int).Exp(y2, exponent, secpP)
	check := new(big.Int).Mul(y, y)
	check.Mod(check, secpP)
	return check.Cmp(y2) == 0
}

var (
	ErrInvalidInvoice     = errors.New("invalid RGB11 invoice")
	ErrInvalidTransport   = errors.New("invalid RGB11 invoice transport")
	ErrInvalidBeneficiary = errors.New("invalid RGB11 invoice beneficiary")
	ErrInvalidState       = errors.New("invalid RGB11 invoice assignment state")
)
