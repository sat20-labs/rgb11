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

const (
	pay2VoutP2PKH  byte = 1
	pay2VoutP2SH   byte = 2
	pay2VoutP2WPKH byte = 3
	pay2VoutP2WSH  byte = 4
	pay2VoutP2TR   byte = 5
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
	// A P2WSH payload is an arbitrary 32-byte script hash. Only P2TR stores an
	// x-only public key and therefore needs to lift to a secp256k1 point.
	if witness[0] == pay2VoutP2TR && !validXOnly(witness[1:]) {
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

// NewWitnessBeneficiary converts a standard Bitcoin output script into the
// Pay2Vout representation used by RGB v0.11 invoices.
func NewWitnessBeneficiary(network ChainNet, pkScript []byte, internalXOnly *[32]byte) (Beneficiary, error) {
	if _, err := ParseChainNet(string(network)); err != nil {
		return Beneficiary{}, err
	}
	var witness [33]byte
	switch {
	case len(pkScript) == 25 && pkScript[0] == 0x76 && pkScript[1] == 0xa9 && pkScript[2] == 0x14 && pkScript[23] == 0x88 && pkScript[24] == 0xac:
		witness[0] = pay2VoutP2PKH
		copy(witness[1:21], pkScript[3:23])
	case len(pkScript) == 23 && pkScript[0] == 0xa9 && pkScript[1] == 0x14 && pkScript[22] == 0x87:
		witness[0] = pay2VoutP2SH
		copy(witness[1:21], pkScript[2:22])
	case len(pkScript) == 22 && pkScript[0] == 0x00 && pkScript[1] == 0x14:
		witness[0] = pay2VoutP2WPKH
		copy(witness[1:21], pkScript[2:22])
	case len(pkScript) == 34 && pkScript[0] == 0x00 && pkScript[1] == 0x20:
		witness[0] = pay2VoutP2WSH
		copy(witness[1:], pkScript[2:])
	case len(pkScript) == 34 && pkScript[0] == 0x51 && pkScript[1] == 0x20 && validXOnly(pkScript[2:]):
		witness[0] = pay2VoutP2TR
		copy(witness[1:], pkScript[2:])
	default:
		return Beneficiary{}, fmt.Errorf("%w: unsupported witness script", ErrInvalidBeneficiary)
	}
	if internalXOnly != nil && !validXOnly(internalXOnly[:]) {
		return Beneficiary{}, fmt.Errorf("%w: invalid internal key", ErrInvalidBeneficiary)
	}
	return Beneficiary{
		Network:       network,
		Kind:          BeneficiaryWitnessVout,
		WitnessVout:   witness,
		InternalXOnly: internalXOnly,
	}, nil
}

// WitnessScript reconstructs the Bitcoin output script requested by a witness
// invoice. It rejects non-witness beneficiaries and non-canonical padding.
func (b Beneficiary) WitnessScript() ([]byte, error) {
	if b.Kind != BeneficiaryWitnessVout {
		return nil, ErrInvalidBeneficiary
	}
	payload := b.WitnessVout[1:]
	switch b.WitnessVout[0] {
	case pay2VoutP2PKH:
		if !allZero(payload[20:]) {
			return nil, ErrInvalidBeneficiary
		}
		return append(append([]byte{0x76, 0xa9, 0x14}, payload[:20]...), 0x88, 0xac), nil
	case pay2VoutP2SH:
		if !allZero(payload[20:]) {
			return nil, ErrInvalidBeneficiary
		}
		return append(append([]byte{0xa9, 0x14}, payload[:20]...), 0x87), nil
	case pay2VoutP2WPKH:
		if !allZero(payload[20:]) {
			return nil, ErrInvalidBeneficiary
		}
		return append([]byte{0x00, 0x14}, payload[:20]...), nil
	case pay2VoutP2WSH:
		return append([]byte{0x00, 0x20}, payload...), nil
	case pay2VoutP2TR:
		if !validXOnly(payload) {
			return nil, ErrInvalidBeneficiary
		}
		return append([]byte{0x51, 0x20}, payload...), nil
	default:
		return nil, ErrInvalidBeneficiary
	}
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
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
