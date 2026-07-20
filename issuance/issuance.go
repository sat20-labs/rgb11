// Package issuance constructs standard RGB 0.11.1-rc.11 contracts without a
// Rust runtime. It starts from the exact official rgb-schemas templates and
// changes only issuer-controlled genesis state and seals.
package issuance

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode"

	"github.com/sat20-labs/rgb11/consignment"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

var (
	ErrInvalidSpec       = errors.New("invalid RGB11 issuance specification")
	ErrUnsupportedSchema = errors.New("RGB11 schema is not enabled for first-release issuance")
)

type ChainNet uint8

const (
	BitcoinMainnet ChainNet = iota
	BitcoinTestnet3
	BitcoinTestnet4
	BitcoinSignet
	BitcoinRegtest
	LiquidMainnet
	LiquidTestnet
	BitcoinSignetCustom
)

// Allocation assigns newly created state to one explicit Bitcoin seal.
// Genesis seals must reference an existing transaction output.
type Allocation struct {
	Seal   seals.GraphBlindSeal
	Amount uint64
}

// Spec is the schema-neutral subset exposed by the frozen rgb-lib wallet API.
// First-release issuance supports the official NIA, IFA and UDA schemas. UDA
// always issues a single non-fractional token.
type Spec struct {
	Kind            schemas.Kind
	Network         ChainNet
	Ticker          string
	Name            string
	Details         string
	Precision       uint8
	Terms           string
	Allocations     []Allocation
	InflationRights []Allocation
	RejectListURL   string
	Timestamp       int64
}

// Result contains the canonical contract armor and its independently decoded
// representation. Callers must still validate Bitcoin UTXO evidence before
// importing wallet state.
type Result struct {
	Armor      string
	ContractID string
	SchemaID   string
	Container  *consignment.Container
}

//go:embed templates/nia.rgba templates/ifa.rgba templates/uda.rgba
var templates embed.FS

func Issue(spec Spec) (*Result, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	path, err := templatePath(spec.Kind)
	if err != nil {
		return nil, err
	}
	raw, err := templates.ReadFile(path)
	if err != nil {
		return nil, err
	}
	template, err := consignment.DecodeArmor(string(raw))
	if err != nil {
		return nil, err
	}
	value := template.Value.Clone()
	schema, _ := value.Field("schema")
	typeSystem, _ := value.Field("types")
	genesis := fieldPointer(&value, "genesis")
	if genesis == nil {
		return nil, ErrInvalidSpec
	}

	registry, err := strict_types.RC11Registry()
	if err != nil {
		return nil, err
	}
	if err := registry.AddTypeSystem("contract", typeSystem); err != nil {
		return nil, err
	}
	if err := setGenesisHeader(genesis, spec); err != nil {
		return nil, err
	}
	if err := setGenesisGlobals(registry, schema, genesis, spec); err != nil {
		return nil, err
	}
	if err := setGenesisAssignments(schema, genesis, spec); err != nil {
		return nil, err
	}

	armor, err := consignment.EncodeArmor(value)
	if err != nil {
		return nil, err
	}
	decoded, err := consignment.DecodeArmor(armor)
	if err != nil {
		return nil, err
	}
	return &Result{Armor: armor, ContractID: decoded.ContractID, SchemaID: decoded.SchemaID, Container: decoded}, nil
}

func validateSpec(spec Spec) error {
	switch spec.Kind {
	case schemas.NIA, schemas.IFA, schemas.UDA:
	default:
		return ErrUnsupportedSchema
	}
	if spec.Network > BitcoinSignetCustom || spec.Precision > 18 || !validText(spec.Name, 1, 40) ||
		(spec.Details != "" && !validText(spec.Details, 1, 255)) ||
		(spec.Terms != "" && !validText(spec.Terms, 1, 65535)) {
		return ErrInvalidSpec
	}
	if !validTicker(spec.Ticker) {
		return ErrInvalidSpec
	}
	if spec.Kind == schemas.UDA {
		if len(spec.Allocations) != 1 || spec.Allocations[0].Amount != 1 || len(spec.InflationRights) != 0 || spec.RejectListURL != "" {
			return ErrInvalidSpec
		}
	} else if spec.Kind != schemas.IFA && len(spec.Allocations) == 0 {
		return ErrInvalidSpec
	}
	if spec.Kind != schemas.IFA && (len(spec.InflationRights) != 0 || spec.RejectListURL != "") {
		return ErrInvalidSpec
	}
	if spec.Kind == schemas.IFA && len(spec.Allocations) == 0 && len(spec.InflationRights) == 0 {
		return ErrInvalidSpec
	}
	if spec.RejectListURL != "" && !validText(spec.RejectListURL, 1, 8000) {
		return ErrInvalidSpec
	}
	seen := make(map[string]struct{})
	for _, allocation := range append(append([]Allocation(nil), spec.Allocations...), spec.InflationRights...) {
		if allocation.Seal.TxID == nil || allocation.Amount == 0 {
			return ErrInvalidSpec
		}
		encoded, err := allocation.Seal.StrictBytes()
		if err != nil {
			return ErrInvalidSpec
		}
		key := string(encoded)
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalidSpec
		}
		seen[key] = struct{}{}
	}
	if _, overflow := sumAmounts(spec.Allocations); overflow {
		return ErrInvalidSpec
	}
	if _, overflow := sumAmounts(spec.InflationRights); overflow {
		return ErrInvalidSpec
	}
	return nil
}

func validTicker(value string) bool {
	if len(value) < 1 || len(value) > 8 {
		return false
	}
	for index, r := range value {
		if r > unicode.MaxASCII || (index == 0 && !unicode.IsLetter(r)) || (index > 0 && !unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func validText(value string, min, max int) bool {
	if len(value) < min || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

func templatePath(kind schemas.Kind) (string, error) {
	switch kind {
	case schemas.NIA:
		return "templates/nia.rgba", nil
	case schemas.IFA:
		return "templates/ifa.rgba", nil
	case schemas.UDA:
		return "templates/uda.rgba", nil
	default:
		return "", ErrUnsupportedSchema
	}
}

func setGenesisHeader(genesis *strict_types.Value, spec Spec) error {
	timestamp := spec.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	timestampValue := fieldPointer(genesis, "timestamp")
	chainNet := fieldPointer(genesis, "chainNet")
	if timestampValue == nil || chainNet == nil {
		return ErrInvalidSpec
	}
	setSigned(timestampValue, timestamp)
	names := [...]string{
		"bitcoinMainnet", "bitcoinTestnet3", "bitcoinTestnet4", "bitcoinSignet",
		"bitcoinRegtest", "liquidMainnet", "liquidTestnet", "bitcoinSignetCustom",
	}
	*chainNet = strict_types.Value{Kind: strict_types.ValueEnum, Name: names[spec.Network], Text: names[spec.Network], Tag: uint8(spec.Network)}
	return nil
}

func setGenesisGlobals(registry *strict_types.Registry, schema strict_types.Value, genesis *strict_types.Value, spec Spec) error {
	issued, _ := sumAmounts(spec.Allocations)
	inflation, _ := sumAmounts(spec.InflationRights)
	maxSupply := issued + inflation

	switch spec.Kind {
	case schemas.NIA, schemas.IFA, schemas.UDA:
		state, semID, err := decodeGlobal(registry, schema, *genesis, 2000)
		if err != nil {
			return err
		}
		if err := mutateAssetSpec(&state, spec); err != nil {
			return err
		}
		if err := putGlobal(registry, schema, genesis, 2000, semID, state); err != nil {
			return err
		}
	}

	if spec.Terms != "" {
		terms, semID, err := decodeGlobal(registry, schema, *genesis, 2001)
		if err != nil {
			return err
		}
		text := fieldPointer(&terms, "text")
		if text == nil {
			return ErrInvalidSpec
		}
		*text = textValue(spec.Terms)
		if err := putGlobal(registry, schema, genesis, 2001, semID, terms); err != nil {
			return err
		}
	}
	if spec.Kind != schemas.UDA {
		if err := putGlobal(registry, schema, genesis, 2010, "", numberValue(issued)); err != nil {
			return err
		}
	}
	if spec.Kind == schemas.IFA {
		if maxSupply < issued {
			return ErrInvalidSpec
		}
		if err := putGlobal(registry, schema, genesis, 2011, "", numberValue(maxSupply)); err != nil {
			return err
		}
		if spec.RejectListURL == "" {
			removeGlobal(genesis, 2012)
		} else if err := putGlobal(registry, schema, genesis, 2012, "", textValue(spec.RejectListURL)); err != nil {
			return err
		}
	}
	return nil
}

func mutateAssetSpec(value *strict_types.Value, spec Spec) error {
	ticker := fieldPointer(value, "ticker")
	name := fieldPointer(value, "name")
	details := fieldPointer(value, "details")
	precision := fieldPointer(value, "precision")
	if ticker == nil || name == nil || details == nil || precision == nil {
		return ErrInvalidSpec
	}
	*ticker = textValue(spec.Ticker)
	*name = textValue(spec.Name)
	*precision = precisionValue(spec.Precision)
	if spec.Details == "" {
		*details = strict_types.Value{Kind: strict_types.ValueUnion, Name: "none", Tag: 0, Inner: &strict_types.Value{Kind: strict_types.ValueUnit}}
	} else {
		inner := textValue(spec.Details)
		*details = strict_types.Value{Kind: strict_types.ValueUnion, Name: "some", Tag: 1, Inner: &inner}
	}
	return nil
}

func setGenesisAssignments(schema strict_types.Value, genesis *strict_types.Value, spec Spec) error {
	groups := map[uint64][]Allocation{4000: spec.Allocations}
	if spec.Kind == schemas.IFA {
		groups[4010] = spec.InflationRights
	}
	assignments := fieldPointer(genesis, "assignments")
	assignments = unwrapPointer(assignments)
	if assignments == nil || assignments.Kind != strict_types.ValueMap {
		return ErrInvalidSpec
	}
	for typeID, allocations := range groups {
		entry := mapEntry(assignments, typeID)
		if entry == nil {
			return fmt.Errorf("%w: assignment %d", ErrInvalidSpec, typeID)
		}
		typed := unwrapPointer(&entry.Value)
		if typed == nil || typed.Kind != strict_types.ValueUnion || typed.Inner == nil {
			return ErrInvalidSpec
		}
		list := unwrapPointer(typed.Inner)
		if list == nil || list.Kind != strict_types.ValueList || len(list.Items) == 0 {
			return ErrInvalidSpec
		}
		template := list.Items[0]
		items := make([]strict_types.Value, 0, len(allocations))
		for _, allocation := range allocations {
			item := template.Clone()
			if err := mutateAssignment(&item, allocation, spec.Kind == schemas.UDA); err != nil {
				return err
			}
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool {
			left := assignmentSecret(items[i])
			right := assignmentSecret(items[j])
			return bytes.Compare(left[:], right[:]) < 0
		})
		list.Items = items
	}
	_ = schema
	return nil
}

func mutateAssignment(value *strict_types.Value, allocation Allocation, uda bool) error {
	value = unwrapPointer(value)
	if value == nil || value.Kind != strict_types.ValueUnion || value.Name != "revealed" || value.Inner == nil {
		return ErrInvalidSpec
	}
	fields := unwrapPointer(value.Inner)
	seal := fieldPointer(fields, "seal")
	state := fieldPointer(fields, "state")
	if seal == nil || state == nil || allocation.Seal.TxID == nil {
		return ErrInvalidSpec
	}
	txid := fieldPointer(seal, "txid")
	vout := fieldPointer(seal, "vout")
	blinding := fieldPointer(seal, "blinding")
	if txid == nil || vout == nil || blinding == nil || !replaceBytes(txid, allocation.Seal.TxID[:]) {
		return ErrInvalidSpec
	}
	setUnsigned(vout, uint64(allocation.Seal.Vout))
	setUnsigned(blinding, allocation.Seal.Blinding)
	if uda {
		data := make([]byte, 12)
		binary.LittleEndian.PutUint32(data[:4], 2)
		binary.LittleEndian.PutUint64(data[4:], 1)
		if !replaceBytes(state, data) {
			return ErrInvalidSpec
		}
	} else if !setFirstUnsigned(state, allocation.Amount) {
		return ErrInvalidSpec
	}
	return nil
}

func assignmentSecret(value strict_types.Value) [32]byte {
	value = value.Unwrap()
	fields := value.Inner.Unwrap()
	seal, _ := fields.Field("seal")
	txid, _ := seal.Field("txid")
	vout, _ := seal.Field("vout")
	blinding, _ := seal.Field("blinding")
	raw, _ := txid.Bytes()
	index, _ := number(vout)
	blind, _ := number(blinding)
	graph, _ := seals.NewGraphBlindSeal(raw, uint32(index), blind)
	secret, _ := graph.Conceal()
	return [32]byte(secret)
}

func decodeGlobal(registry *strict_types.Registry, schema, genesis strict_types.Value, typeID uint64) (strict_types.Value, string, error) {
	semID, err := globalSemanticID(schema, typeID)
	if err != nil {
		return strict_types.Value{}, "", err
	}
	globals, _ := genesis.Field("globals")
	entry := mapEntryValue(globals, typeID)
	if entry == nil {
		return strict_types.Value{}, "", ErrInvalidSpec
	}
	list := entry.Value.Unwrap()
	if list.Kind != strict_types.ValueList || len(list.Items) != 1 {
		return strict_types.Value{}, "", ErrInvalidSpec
	}
	raw, ok := list.Items[0].Bytes()
	if !ok {
		return strict_types.Value{}, "", ErrInvalidSpec
	}
	value, err := registry.DecodeSemantic("contract", semID, raw)
	return value, semID, err
}

func putGlobal(registry *strict_types.Registry, schema strict_types.Value, genesis *strict_types.Value, typeID uint64, semID string, value strict_types.Value) error {
	if semID == "" {
		var err error
		semID, err = globalSemanticID(schema, typeID)
		if err != nil {
			return err
		}
	}
	raw, err := registry.EncodeSemantic("contract", semID, value)
	if err != nil {
		return err
	}
	globals := fieldPointer(genesis, "globals")
	globals = unwrapPointer(globals)
	if globals == nil || globals.Kind != strict_types.ValueMap || len(globals.Entries) == 0 {
		return ErrInvalidSpec
	}
	entry := mapEntry(globals, typeID)
	if entry == nil {
		key, err := schemaMapKey(schema, "globalTypes", typeID)
		if err != nil {
			return err
		}
		entryValue := globals.Entries[0].Value.Clone()
		globals.Entries = append(globals.Entries, strict_types.Entry{Key: key, Value: entryValue})
		sort.Slice(globals.Entries, func(i, j int) bool {
			left, _ := number(globals.Entries[i].Key)
			right, _ := number(globals.Entries[j].Key)
			return left < right
		})
		entry = mapEntry(globals, typeID)
	}
	list := unwrapPointer(&entry.Value)
	if list == nil || list.Kind != strict_types.ValueList || len(list.Items) == 0 {
		return ErrInvalidSpec
	}
	state := list.Items[0].Clone()
	if !replaceBytes(&state, raw) {
		return ErrInvalidSpec
	}
	list.Items = []strict_types.Value{state}
	return nil
}

func removeGlobal(genesis *strict_types.Value, typeID uint64) {
	globals := unwrapPointer(fieldPointer(genesis, "globals"))
	if globals == nil || globals.Kind != strict_types.ValueMap {
		return
	}
	filtered := globals.Entries[:0]
	for _, entry := range globals.Entries {
		id, _ := number(entry.Key)
		if id != typeID {
			filtered = append(filtered, entry)
		}
	}
	globals.Entries = filtered
}

func globalSemanticID(schema strict_types.Value, typeID uint64) (string, error) {
	globalTypes, ok := schema.Field("globalTypes")
	if !ok {
		return "", ErrInvalidSpec
	}
	entry := mapEntryValue(globalTypes, typeID)
	if entry == nil {
		return "", ErrInvalidSpec
	}
	details := entry.Value.Unwrap()
	stateSchema, ok := details.Field("globalStateSchema")
	if !ok {
		return "", ErrInvalidSpec
	}
	semValue, ok := stateSchema.Unwrap().Field("semId")
	raw, rawOK := semValue.Bytes()
	if !ok || !rawOK || len(raw) != 32 {
		return "", ErrInvalidSpec
	}
	const alphabet = "0123456789abcdef"
	text := make([]byte, 64)
	for index, b := range raw {
		text[index*2] = alphabet[b>>4]
		text[index*2+1] = alphabet[b&15]
	}
	return string(text), nil
}

func schemaMapKey(schema strict_types.Value, field string, typeID uint64) (strict_types.Value, error) {
	values, ok := schema.Field(field)
	if !ok {
		return strict_types.Value{}, ErrInvalidSpec
	}
	entry := mapEntryValue(values, typeID)
	if entry == nil {
		return strict_types.Value{}, ErrInvalidSpec
	}
	return entry.Key.Clone(), nil
}

func fieldPointer(value *strict_types.Value, name string) *strict_types.Value {
	value = unwrapPointer(value)
	if value == nil || value.Kind != strict_types.ValueStruct {
		return nil
	}
	for index := range value.Fields {
		if value.Fields[index].Name == name {
			return &value.Fields[index].Value
		}
	}
	return nil
}

func unwrapPointer(value *strict_types.Value) *strict_types.Value {
	for value != nil && value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		value = &value.Items[0]
	}
	return value
}

func mapEntry(value *strict_types.Value, typeID uint64) *strict_types.Entry {
	value = unwrapPointer(value)
	if value == nil || value.Kind != strict_types.ValueMap {
		return nil
	}
	for index := range value.Entries {
		id, ok := number(value.Entries[index].Key)
		if ok && id == typeID {
			return &value.Entries[index]
		}
	}
	return nil
}

func mapEntryValue(value strict_types.Value, typeID uint64) *strict_types.Entry {
	return mapEntry(&value, typeID)
}

func number(value strict_types.Value) (uint64, bool) {
	value = value.Unwrap()
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return number(*value.Inner)
	}
	return value.Uint64()
}

func replaceBytes(value *strict_types.Value, raw []byte) bool {
	if value == nil {
		return false
	}
	if value.Kind == strict_types.ValueBytes {
		value.Raw = append([]byte(nil), raw...)
		value.Encoded = nil
		return true
	}
	if value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		return replaceBytes(&value.Items[0], raw)
	}
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return replaceBytes(value.Inner, raw)
	}
	return false
}

func setFirstUnsigned(value *strict_types.Value, amount uint64) bool {
	if value == nil {
		return false
	}
	if value.Kind == strict_types.ValueNumber {
		setUnsigned(value, amount)
		return true
	}
	if value.Kind == strict_types.ValueTuple && len(value.Items) == 1 {
		return setFirstUnsigned(&value.Items[0], amount)
	}
	if value.Kind == strict_types.ValueUnion && value.Inner != nil {
		return setFirstUnsigned(value.Inner, amount)
	}
	return false
}

func setUnsigned(value *strict_types.Value, amount uint64) {
	value = unwrapPointer(value)
	if value == nil {
		return
	}
	value.Kind = strict_types.ValueNumber
	value.Unsigned = &amount
	value.Signed = nil
	value.Raw = nil
	value.Encoded = nil
}

func setSigned(value *strict_types.Value, amount int64) {
	value = unwrapPointer(value)
	if value == nil {
		return
	}
	value.Kind = strict_types.ValueNumber
	value.Signed = &amount
	value.Unsigned = nil
	value.Raw = nil
	value.Encoded = nil
}

func textValue(text string) strict_types.Value {
	return strict_types.Value{Kind: strict_types.ValueString, Text: text, Raw: []byte(text)}
}

func numberValue(value uint64) strict_types.Value {
	return strict_types.Value{Kind: strict_types.ValueNumber, Primitive: 8, Unsigned: &value}
}

func precisionValue(precision uint8) strict_types.Value {
	names := [...]string{
		"indivisible", "deci", "centi", "milli", "deciMilli", "centiMilli", "micro", "deciMicro", "centiMicro",
		"nano", "deciNano", "centiNano", "pico", "deciPico", "centiPico", "femto", "deciFemto", "centiFemto", "atto",
	}
	return strict_types.Value{Kind: strict_types.ValueEnum, Name: names[precision], Text: names[precision], Tag: precision}
}

func sumAmounts(allocations []Allocation) (uint64, bool) {
	var total uint64
	for _, allocation := range allocations {
		if ^uint64(0)-total < allocation.Amount {
			return 0, true
		}
		total += allocation.Amount
	}
	return total, false
}
