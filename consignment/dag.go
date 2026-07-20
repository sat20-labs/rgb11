package consignment

import (
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/strict_types"
)

// OperationDAG returns the disclosed client-side history as child Opout to
// parent Opout edges. Each output created by a transition inherits all of the
// transition inputs. Genesis outputs are roots and therefore have no parents.
func (c *Container) OperationDAG() (map[operations.Opout][]operations.Opout, error) {
	if c == nil || !c.StructuralValid || !c.GenesisValid {
		return nil, ErrContainerType
	}
	dag := make(map[operations.Opout][]operations.Opout)
	genesis, ok := c.Value.Field("genesis")
	if !ok {
		return nil, operations.ErrInvalidGenesis
	}
	genesisCommitment, err := operations.CommitGenesis(genesis)
	if err != nil {
		return nil, err
	}
	if err := addDAGOutputs(dag, genesisCommitment.OperationID, genesis, nil); err != nil {
		return nil, err
	}
	bundles, ok := c.Value.Field("bundles")
	bundles = bundles.Unwrap()
	if !ok || bundles.Kind != strict_types.ValueList {
		return nil, operations.ErrInvalidBundle
	}
	for _, witnessBundle := range bundles.Items {
		bundleValue, ok := witnessBundle.Unwrap().Field("bundle")
		if !ok {
			return nil, operations.ErrInvalidBundle
		}
		bundle, err := operations.CommitBundle(bundleValue)
		if err != nil {
			return nil, err
		}
		for _, transition := range bundle.Transitions {
			commitment, err := operations.CommitTransition(transition)
			if err != nil {
				return nil, err
			}
			refs, err := transitionInputs(transition)
			if err != nil {
				return nil, err
			}
			parents := make([]operations.Opout, 0, len(refs))
			for _, ref := range refs {
				parents = append(parents, inputRefOpout(ref))
			}
			if err := addDAGOutputs(dag, commitment.OperationID, transition, parents); err != nil {
				return nil, err
			}
		}
	}
	return dag, nil
}

func addDAGOutputs(dag map[operations.Opout][]operations.Opout, operationID [32]byte,
	operation strict_types.Value, parents []operations.Opout) error {
	outputs, err := schemas.RevealedOutputs(operation)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		opout := operations.Opout{
			Operation: consensus.OperationID(operationID),
			Type:      operations.AssignmentType(output.AssignmentType),
			Number:    output.Index,
		}
		dag[opout] = append([]operations.Opout(nil), parents...)
	}
	return nil
}

func inputRefOpout(ref schemas.InputRef) operations.Opout {
	return operations.Opout{
		Operation: consensus.OperationID(ref.OperationID),
		Type:      operations.AssignmentType(ref.AssignmentType),
		Number:    ref.Index,
	}
}
