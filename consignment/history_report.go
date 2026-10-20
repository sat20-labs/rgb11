package consignment

import (
	"bytes"
	"github.com/btcsuite/btcd/wire"
	"github.com/sat20-labs/rgb11/schemas"
	"sort"
)

// ValidatedHistoricalState is an exact allocation from the selected,
// core-validated RGB graph. WitnessState describes the transaction that
// created it; this material report does not establish current Bitcoin
// ownership or spendability.
type ValidatedHistoricalState struct {
	Reference      schemas.InputRef
	State          schemas.ResolvedInput
	Outpoint       Outpoint
	SealDisclosure []byte
	SealBlinding   uint64
	WitnessTxPtr   bool
	WitnessTxID    [32]byte
	WitnessState   WitnessState
	CarrierBinding CarrierBinding
}

// ValidatedConsumer is one exact RGB Opout consumed by a fully validated
// transition and bound to the Bitcoin witness bundle that closes it. It is
// reported only after the entire container validates successfully.
type ValidatedConsumer struct {
	ContractID          string
	Input               schemas.InputRef
	ConsumerOperationID [32]byte
	BundleID            [32]byte
	WitnessTxID         [32]byte
	WitnessState        WitnessState
}

// HistoryReport adds material facts without changing the original Validation
// result or the behavior of Validate, ValidateHistory, or ValidatePrepared.
// It never grants current ownership; failed report calls return no facts.
type HistoryReport struct {
	Validation
	ValidatedConsumers []ValidatedConsumer
	HistoricalStates   []ValidatedHistoricalState
	PreparedStates     []ValidatedState
}

func (c *Container) ValidateHistoryReport(resolver BitcoinResolver) (HistoryReport, error) {
	return c.historyReport(resolver, false)
}

// ValidatePreparedHistoryReport validates the embedded branch as material.
// Its inputs need not still be unspent. Callers must establish current-chain
// evidence separately before projecting ownership or classifying a reorg.
func (c *Container) ValidatePreparedHistoryReport(resolver BitcoinResolver) (HistoryReport, error) {
	return c.historyReport(resolver, true)
}

func (c *Container) historyReport(resolver BitcoinResolver, prepared bool) (HistoryReport, error) {
	collector := &historyReportCollector{selected: make(map[[32]byte]bool),
		witnessStates: make(map[[32]byte]WitnessState)}
	report, err := c.validateWithHistoryReport(resolver, prepared, true, collector)
	if err != nil {
		return HistoryReport{}, err
	}
	collector.report.Validation = report
	return collector.report, nil
}

type historicalConsumption struct {
	edge   ValidatedConsumer
	output validatedOutput
}

type historyReportCollector struct {
	report        HistoryReport
	selected      map[[32]byte]bool
	witnessStates map[[32]byte]WitnessState
	consumed      []historicalConsumption
}

// Called only after the existing consensus validator accepted the transition,
// and before its exact consumed InputRefs are removed from the working map.
func (h *historyReportCollector) recordConsumption(contractID string, inputs []schemas.InputRef,
	states map[schemas.InputRef]validatedOutput, operation, bundle, witness [32]byte, status WitnessState) {
	h.witnessStates[witness] = status
	for _, input := range inputs {
		h.consumed = append(h.consumed, historicalConsumption{
			edge: ValidatedConsumer{ContractID: contractID, Input: input, ConsumerOperationID: operation,
				BundleID: bundle, WitnessTxID: witness, WitnessState: status}, output: states[input]})
	}
}

func (h *historyReportCollector) finish(states map[schemas.InputRef]validatedOutput,
	prepared map[[32]byte]*wire.MsgTx, complete bool) error {
	seen := make(map[schemas.InputRef]bool)
	add := func(ref schemas.InputRef, output validatedOutput) {
		if !complete || seen[ref] {
			return
		}
		seen[ref] = true
		h.report.HistoricalStates = append(h.report.HistoricalStates, ValidatedHistoricalState{
			Reference: ref, State: output.state, Outpoint: output.outpoint,
			SealDisclosure: append([]byte(nil), output.sealDisclosure...), SealBlinding: output.sealBlinding,
			WitnessTxPtr: output.witnessTxPtr, WitnessTxID: output.witnessTxID,
			WitnessState: h.witnessStates[output.witnessTxID], CarrierBinding: cloneCarrierBinding(output.carrierBinding)})
	}
	for _, consumed := range h.consumed {
		h.report.ValidatedConsumers = append(h.report.ValidatedConsumers, consumed.edge)
		if !h.selected[consumed.edge.ConsumerOperationID] {
			continue
		}
		add(consumed.edge.Input, consumed.output)
	}
	for ref, output := range states {
		if !h.selected[ref.OperationID] {
			continue
		}
		add(ref, output)
		tx := prepared[output.outpoint.TxID]
		if tx == nil {
			continue
		}
		if uint64(output.outpoint.Vout) >= uint64(len(tx.TxOut)) {
			return ErrOutpointUnknown
		}
		txout := tx.TxOut[output.outpoint.Vout]
		h.report.PreparedStates = append(h.report.PreparedStates, ValidatedState{
			Reference: ref, State: output.state, Outpoint: output.outpoint,
			SealDisclosure: append([]byte(nil), output.sealDisclosure...), SealBlinding: output.sealBlinding,
			WitnessTxPtr: output.witnessTxPtr, WitnessTxID: output.witnessTxID,
			CarrierBinding: cloneCarrierBinding(output.carrierBinding), WitnessPrepared: true,
			OutputValue: txout.Value, OutputScript: append([]byte(nil), txout.PkScript...)})
	}
	sort.Slice(h.report.HistoricalStates, func(i, j int) bool {
		left, right := h.report.HistoricalStates[i].Reference, h.report.HistoricalStates[j].Reference
		if cmp := bytes.Compare(left.OperationID[:], right.OperationID[:]); cmp != 0 {
			return cmp < 0
		}
		if left.AssignmentType != right.AssignmentType {
			return left.AssignmentType < right.AssignmentType
		}
		return left.Index < right.Index
	})
	return nil
}
