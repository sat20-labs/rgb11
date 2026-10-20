package consignment

import (
	"errors"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
	"testing"
)

func TestHistoryCollectorPartialSelectionRetainsVerifiedConsumers(t *testing.T) {
	ref := schemas.InputRef{OperationID: [32]byte{1}, AssignmentType: 4000, Index: 3}
	h := &historyReportCollector{selected: make(map[[32]byte]bool), witnessStates: make(map[[32]byte]WitnessState)}
	h.recordConsumption("verified-contract", []schemas.InputRef{ref},
		map[schemas.InputRef]validatedOutput{ref: {}}, [32]byte{2}, [32]byte{3}, [32]byte{4}, WitnessTentative)
	if err := h.finish(nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if len(h.report.ValidatedConsumers) != 1 || h.report.ValidatedConsumers[0].Input != ref ||
		len(h.report.HistoricalStates) != 0 || len(h.report.CurrentStates) != 0 {
		t.Fatal("uncertain terminal selection lost a verified full Opout consumer or invented complete history")
	}
}

func TestValidateHistoryReportReturnsConsumersOnlyAfterCompleteValidation(t *testing.T) {
	container, resolver := loadValidationVector(t)
	report, err := container.ValidateHistoryReport(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ValidatedConsumers) != 1 || len(report.CurrentStates) != 0 ||
		report.ConsensusValid || container.ConsensusValid {
		t.Fatalf("history report leaked projection or omitted its consumer edge: %+v", report)
	}
	if len(report.HistoricalStates) != 2 {
		t.Fatalf("history report states=%d, want exact consumed input plus selected terminal output: %+v", len(report.HistoricalStates), report.HistoricalStates)
	}
	consumer := report.ValidatedConsumers[0]
	var sawConsumedInput, sawTerminal bool
	for _, state := range report.HistoricalStates {
		if state.Reference == consumer.Input {
			sawConsumedInput = true
			if state.WitnessState != WitnessUnknown {
				t.Fatalf("genesis allocation inherited a transition witness: %+v", state)
			}
		}
		if state.Reference.OperationID == consumer.ConsumerOperationID {
			sawTerminal = true
			if state.WitnessTxID != resolver.txid || state.WitnessState != WitnessTentative {
				t.Fatalf("terminal history omitted its creating witness fact: %+v", state)
			}
		}
	}
	if !sawConsumedInput || !sawTerminal {
		t.Fatalf("historical report did not close the selected Opout graph: input=%v terminal=%v", sawConsumedInput, sawTerminal)
	}

	container, resolver = loadValidationVector(t)
	resolver.unknownWitness = true
	failed, err := container.ValidateHistoryReport(resolver)
	if err == nil || len(failed.ValidatedConsumers) != 0 || len(failed.HistoricalStates) != 0 ||
		len(failed.CurrentStates) != 0 || len(failed.PreparedStates) != 0 {
		t.Fatalf("invalid history exposed typed graph facts: report=%+v err=%v", failed, err)
	}
}

func TestValidateContractHistoryReportsConsumedBundleInputs(t *testing.T) {
	container, resolver := loadValidationVector(t)
	unwrapPointer(fieldPointer(&container.Value, "transfer")).Name = "false"

	report, err := container.ValidateHistoryReport(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ValidatedConsumers) != 1 {
		t.Fatalf("contract history consumers=%d, want the validated bundle edge", len(report.ValidatedConsumers))
	}
	input := report.ValidatedConsumers[0].Input
	for _, state := range report.HistoricalStates {
		if state.Reference == input {
			resolver.currentSpent = true
			if _, err := container.Validate(resolver); !errors.Is(err, ErrOutpointSpend) {
				t.Fatalf("ordinary contract projection must still reject a spent bundled input: %v", err)
			}
			return
		}
	}
	t.Fatalf("contract history omitted consumed bundle input Opout %x/%d/%d; states=%+v",
		input.OperationID, input.AssignmentType, input.Index, report.HistoricalStates)
}

func TestValidatePreparedHistoryReportSeparatesCurrentInputOwnership(t *testing.T) {
	container, resolver := loadValidationVector(t)
	resolver.unknownWitness = true
	// The previous allocation is now spent, so this witness cannot be prepared
	// as a current spend. Its embedded transaction and transition graph remain
	// structurally/consensus verifiable for a reorg-branch comparison.
	report, err := container.ValidatePreparedHistoryReport(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if report.PreparedWitnesses != 1 || len(report.ValidatedConsumers) != 1 || len(report.PreparedStates) != 1 ||
		len(report.CurrentStates) != 0 ||
		report.ConsensusValid || report.PreparedValid {
		t.Fatalf("prepared historical report leaked current ownership or omitted its edge: %+v", report)
	}
	prepared := report.PreparedStates[0]
	if !prepared.WitnessPrepared || prepared.Outpoint.TxID != resolver.txid ||
		prepared.Reference.OperationID != report.ValidatedConsumers[0].ConsumerOperationID ||
		prepared.OutputValue <= 0 {
		t.Fatalf("prepared history omitted its core-validated witness output: %+v", prepared)
	}
	var foundPreparedHistory bool
	for _, state := range report.HistoricalStates {
		if state.Reference.OperationID == report.ValidatedConsumers[0].ConsumerOperationID {
			foundPreparedHistory = state.WitnessTxID == resolver.txid && state.WitnessState == WitnessPrepared
		}
	}
	if !foundPreparedHistory {
		t.Fatalf("historical report lost the embedded WitnessPrepared fact: %+v", report.HistoricalStates)
	}
	if _, err := container.ValidatePrepared(resolver); !errors.Is(err, ErrOutpointSpend) {
		t.Fatalf("current prepared validation accepted a spent input: %v", err)
	}
}

func TestHistoryReportFailureReturnsNoMaterial(t *testing.T) {
	container, resolver := loadValidationVector(t)
	resolver.unknownWitness = true
	for i := range container.Value.Fields {
		if container.Value.Fields[i].Name == "terminals" {
			container.Value.Fields[i].Value = strict_types.Value{Kind: strict_types.ValueUnit}
			break
		}
	}
	report, err := container.ValidatePreparedHistoryReport(resolver)
	if !errors.Is(err, ErrContainerType) || len(report.HistoricalStates) != 0 || len(report.ValidatedConsumers) != 0 || len(report.PreparedStates) != 0 || len(report.CurrentStates) != 0 {
		t.Fatalf("failed report leaked material: %+v %v", report, err)
	}
}
