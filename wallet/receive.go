// Package wallet contains the RGB11 wallet state machine without depending on
// a concrete database, Bitcoin backend or SAT20 Wallet SDK model.
package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/invoicing"
	"github.com/sat20-labs/rgb11/relay"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/storage"
)

const ReceiveVersion uint32 = 1

type ReceiveMode string

const (
	ReceiveBlind   ReceiveMode = "blind"
	ReceiveWitness ReceiveMode = "witness"
)

var (
	ErrInvalidReceive = errors.New("invalid RGB11 receive request")
	ErrReceiveState   = errors.New("invalid RGB11 receive state transition")
)

type ReceiveStatus string

const (
	ReceivePrepared ReceiveStatus = "PREPARED"
	ReceiveRelayed  ReceiveStatus = "RELAYED"
	ReceiveAccepted ReceiveStatus = "ACCEPTED"
	ReceiveSettled  ReceiveStatus = "SETTLED"
	ReceiveFailed   ReceiveStatus = "FAILED"
)

// ReceiveRequest persists the seal reveal and relay secrets before Invoice is
// returned to the caller. This ordering prevents displaying an invoice whose
// concealed seal cannot later be recovered.
type ReceiveRequest struct {
	Version       uint32               `json:"version"`
	Mode          ReceiveMode          `json:"mode,omitempty"`
	RequestID     string               `json:"request_id"`
	RecipientID   string               `json:"recipient_id"`
	Seal          seals.GraphBlindSeal `json:"seal"`
	WitnessScript []byte               `json:"witness_script,omitempty"`
	Invoice       string               `json:"invoice"`
	RelayKey      string               `json:"relay_key"`
	AckKey        string               `json:"ack_key"`
	CreatedAt     int64                `json:"created_at"`
	Expiry        int64                `json:"expiry"`
	Status        ReceiveStatus        `json:"status"`
	TransferID    string               `json:"transfer_id,omitempty"`
	ObjectHash    string               `json:"object_hash,omitempty"`
	WitnessTxID   string               `json:"witness_txid,omitempty"`
	FailureCode   string               `json:"failure_code,omitempty"`
}

type ReceiveParams struct {
	Mode           ReceiveMode
	ContractID     string
	SchemaID       string
	Network        invoicing.ChainNet
	Amount         *uint64
	AssignmentName string
	RecipientID    string
	WitnessVout    uint32
	WitnessScript  []byte
	InternalXOnly  *[32]byte
	Expiry         int64
	Transports     []invoicing.Transport
}

type Engine struct {
	store storage.Store
	now   func() time.Time
}

func NewEngine(store storage.Store) (*Engine, error) {
	if store == nil {
		return nil, errors.New("RGB11 wallet store is required")
	}
	return &Engine{store: store, now: time.Now}, nil
}

func (e *Engine) CreateReceive(params ReceiveParams) (*ReceiveRequest, error) {
	if params.RecipientID == "" || params.Expiry <= e.now().Unix() {
		return nil, ErrInvalidReceive
	}
	if _, err := invoicing.ParseChainNet(string(params.Network)); err != nil {
		return nil, err
	}
	mode := params.Mode
	if mode == "" {
		mode = ReceiveBlind
	}
	var seal seals.GraphBlindSeal
	var beneficiary invoicing.Beneficiary
	switch mode {
	case ReceiveBlind:
		var err error
		seal, err = seals.RandomWitnessBlindSeal(params.WitnessVout)
		if err != nil {
			return nil, err
		}
		secretSeal, err := seal.Conceal()
		if err != nil {
			return nil, err
		}
		beneficiary = invoicing.Beneficiary{Network: params.Network, Kind: invoicing.BeneficiaryBlindedSeal, BlindedSeal: secretSeal}
	case ReceiveWitness:
		var err error
		beneficiary, err = invoicing.NewWitnessBeneficiary(params.Network, params.WitnessScript, params.InternalXOnly)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidReceive
	}
	invoice := invoicing.Invoice{
		Transports:  append([]invoicing.Transport(nil), params.Transports...),
		Beneficiary: beneficiary,
		Expiry:      &params.Expiry,
		UnknownQuery: []invoicing.QueryParam{
			{Key: "sat20_recipient", Value: params.RecipientID},
			{Key: "sat20_vout", Value: fmt.Sprintf("%d", params.WitnessVout)},
		},
	}
	if params.ContractID != "" {
		contract, err := consensus.ParseContractID(params.ContractID)
		if err != nil {
			return nil, err
		}
		invoice.Contract = &contract
	}
	if params.SchemaID != "" {
		schema, err := baid64.Decode32(params.SchemaID, baid64.SchemaIDOptions())
		if err != nil {
			return nil, err
		}
		invoice.Schema = &schema
	}
	if params.Amount != nil {
		state := invoicing.InvoiceState{Kind: invoicing.StateAmount, Amount: invoicing.Amount(*params.Amount)}
		invoice.Assignment = &state
	}
	if params.AssignmentName != "" {
		invoice.AssignmentName = params.AssignmentName
	}
	relayKey, ackKey, err := relay.NewTemporaryKeys()
	if err != nil {
		return nil, err
	}
	invoice.UnknownQuery = append(invoice.UnknownQuery,
		invoicing.QueryParam{Key: "sat20_relay", Value: relayKey},
		invoicing.QueryParam{Key: "sat20_ack", Value: ackKey},
	)
	if err := invoice.Validate(e.now().Unix()); err != nil {
		return nil, err
	}
	requestID, err := randomID()
	if err != nil {
		return nil, err
	}
	request := &ReceiveRequest{
		Version: ReceiveVersion, Mode: mode, RequestID: requestID, RecipientID: params.RecipientID,
		Seal: seal, WitnessScript: append([]byte(nil), params.WitnessScript...), Invoice: invoice.String(), RelayKey: relayKey, AckKey: ackKey,
		CreatedAt: e.now().Unix(), Expiry: params.Expiry, Status: ReceivePrepared,
	}
	if err := e.putReceive(request); err != nil {
		return nil, err
	}
	return request, nil
}

func (e *Engine) LoadReceive(requestID string) (*ReceiveRequest, error) {
	if requestID == "" {
		return nil, ErrInvalidReceive
	}
	raw, err := e.store.Get([]byte("wallet/receive/" + requestID))
	if err != nil {
		return nil, err
	}
	var request ReceiveRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	return &request, nil
}

func (e *Engine) MarkRelayAccepted(requestID, transferID, objectHash string) error {
	request, err := e.LoadReceive(requestID)
	if err != nil {
		return err
	}
	if request.Status != ReceivePrepared && request.Status != ReceiveRelayed {
		return ErrReceiveState
	}
	decoded, err := hex.DecodeString(objectHash)
	if err != nil || len(decoded) != 32 || transferID == "" {
		return ErrInvalidReceive
	}
	request.Status = ReceiveAccepted
	request.TransferID = transferID
	request.ObjectHash = objectHash
	return e.putReceive(request)
}

func (e *Engine) putReceive(request *ReceiveRequest) error {
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	tx, err := e.store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tx.Put([]byte("wallet/receive/"+request.RequestID), encoded); err != nil {
		return err
	}
	return tx.Commit()
}

func randomID() (string, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate RGB11 identifier: %w", err)
	}
	return hex.EncodeToString(entropy[:]), nil
}
