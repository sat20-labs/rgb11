package invoicing

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	strict "github.com/sat20-labs/rgb11/strict_types"
)

// Upstream-Repository: rgb-protocol/rgb-ops
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 5308b9d46c91857513ff5be2459992264687632b
// Upstream-File: invoice/src/parse.rs
// Translation-Revision: 1

const omitted = "~"

type QueryParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Invoice struct {
	Transports     []Transport           `json:"transports"`
	Contract       *consensus.ContractID `json:"contract,omitempty"`
	Schema         *[32]byte             `json:"schema,omitempty"`
	AssignmentName string                `json:"assignment_name,omitempty"`
	Assignment     *InvoiceState         `json:"assignment,omitempty"`
	Beneficiary    Beneficiary           `json:"beneficiary"`
	Expiry         *int64                `json:"expiry,omitempty"`
	UnknownQuery   []QueryParam          `json:"unknown_query,omitempty"`
}

func Parse(value string) (*Invoice, error) {
	if !strings.HasPrefix(value, "rgb:") {
		return nil, ErrInvalidInvoice
	}
	remainder := strings.TrimPrefix(value, "rgb:")
	if strings.HasPrefix(remainder, "//") || strings.HasPrefix(remainder, "/") || strings.Contains(remainder, "#") {
		return nil, ErrInvalidInvoice
	}
	path, query, hasQuery := strings.Cut(remainder, "?")
	parts := strings.Split(path, "/")
	if len(parts) != 4 {
		return nil, ErrInvalidInvoice
	}
	invoice := &Invoice{}
	if parts[0] != omitted {
		id, err := consensus.ParseContractID(parts[0])
		if err != nil {
			return nil, fmt.Errorf("%w: contract: %v", ErrInvalidInvoice, err)
		}
		invoice.Contract = &id
	}
	if parts[1] != omitted {
		schema, err := baid64.Decode32(parts[1], baid64.SchemaIDOptions())
		if err != nil {
			return nil, fmt.Errorf("%w: schema: %v", ErrInvalidInvoice, err)
		}
		invoice.Schema = &schema
	}
	if parts[2] != omitted {
		state, err := ParseInvoiceState(parts[2])
		if err != nil {
			return nil, err
		}
		invoice.Assignment = &state
	}
	beneficiary, err := ParseBeneficiary(parts[3])
	if err != nil {
		return nil, err
	}
	invoice.Beneficiary = beneficiary

	params, err := parseQuery(query, hasQuery)
	if err != nil {
		return nil, err
	}
	for _, param := range params {
		switch param.Key {
		case "assignment_name":
			if !strict.ValidFieldName(param.Value) {
				return nil, fmt.Errorf("%w: assignment name %q", ErrInvalidInvoice, param.Value)
			}
			invoice.AssignmentName = param.Value
		case "expiry":
			expiry, err := strconv.ParseInt(param.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: expiry %q", ErrInvalidInvoice, param.Value)
			}
			invoice.Expiry = &expiry
		case "endpoints":
			if param.Value == "" {
				return nil, ErrInvalidTransport
			}
			for _, raw := range strings.Split(param.Value, ",") {
				transport, err := ParseTransport(raw)
				if err != nil {
					return nil, err
				}
				invoice.Transports = append(invoice.Transports, transport)
			}
		default:
			invoice.UnknownQuery = upsertParam(invoice.UnknownQuery, param)
		}
	}
	return invoice, nil
}

func (i Invoice) String() string {
	contract := omitted
	if i.Contract != nil {
		contract = strings.TrimPrefix(i.Contract.String(), "rgb:")
	}
	schema := omitted
	if i.Schema != nil {
		opts := baid64.SchemaIDOptions()
		opts.Prefix, opts.Chunking, opts.Mnemonic = false, false, false
		schema, _ = baid64.Encode32(*i.Schema, opts)
	}
	assignment := omitted
	if i.Assignment != nil {
		assignment = i.Assignment.String()
	}
	var b strings.Builder
	b.WriteString("rgb:")
	b.WriteString(contract)
	b.WriteByte('/')
	b.WriteString(schema)
	b.WriteByte('/')
	b.WriteString(assignment)
	b.WriteByte('/')
	b.WriteString(i.Beneficiary.String())
	params := make([]QueryParam, 0, 3+len(i.UnknownQuery))
	if i.AssignmentName != "" {
		params = append(params, QueryParam{Key: "assignment_name", Value: i.AssignmentName})
	}
	if i.Expiry != nil {
		params = append(params, QueryParam{Key: "expiry", Value: strconv.FormatInt(*i.Expiry, 10)})
	}
	if len(i.Transports) > 0 {
		values := make([]string, 0, len(i.Transports))
		for _, transport := range i.Transports {
			values = append(values, transport.String())
		}
		params = append(params, QueryParam{Key: "endpoints", Value: strings.Join(values, ",")})
	}
	params = append(params, i.UnknownQuery...)
	for index, param := range params {
		if index == 0 {
			b.WriteByte('?')
		} else {
			b.WriteByte('&')
		}
		b.WriteString(percentEncode(param.Key))
		b.WriteByte('=')
		b.WriteString(percentEncode(param.Value))
	}
	return b.String()
}

func parseQuery(query string, present bool) ([]QueryParam, error) {
	if !present {
		return nil, nil
	}
	if query == "" {
		return nil, nil
	}
	params := make([]QueryParam, 0)
	for _, raw := range strings.Split(query, "&") {
		keyRaw, valueRaw, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("%w: query %q", ErrInvalidInvoice, raw)
		}
		key, err := url.PathUnescape(keyRaw)
		if err != nil {
			return nil, ErrInvalidInvoice
		}
		value, err := url.PathUnescape(valueRaw)
		if err != nil {
			return nil, ErrInvalidInvoice
		}
		params = upsertParam(params, QueryParam{Key: key, Value: value})
	}
	return params, nil
}

func upsertParam(params []QueryParam, next QueryParam) []QueryParam {
	for index := range params {
		if params[index].Key == next.Key {
			params[index].Value = next.Value
			return params
		}
	}
	return append(params, next)
}

func percentEncode(value string) string {
	const hexChars = "0123456789ABCDEF"
	var b strings.Builder
	for _, char := range []byte(value) {
		if char < 0x20 || char == 0x7f || char >= 0x80 || strings.ContainsRune(" \"#<>[]&=", rune(char)) {
			b.WriteByte('%')
			b.WriteByte(hexChars[char>>4])
			b.WriteByte(hexChars[char&15])
		} else {
			b.WriteByte(char)
		}
	}
	return b.String()
}

func (i Invoice) Validate(now int64) error {
	if i.Beneficiary.String() == "" {
		return ErrInvalidBeneficiary
	}
	if i.Expiry != nil && now >= *i.Expiry {
		return errors.New("RGB11 invoice expired")
	}
	if i.AssignmentName != "" && !strict.ValidFieldName(i.AssignmentName) {
		return ErrInvalidInvoice
	}
	return nil
}
