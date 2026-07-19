package consignment

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/wire"
	"github.com/sat20-labs/rgb11/anchors"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/schemas"
	"github.com/sat20-labs/rgb11/seals"
	"github.com/sat20-labs/rgb11/strict_types"
)

var (
	ErrBundleAnchor        = errors.New("RGB11 bundle anchor is invalid")
	ErrSealNotClosed       = errors.New("RGB11 single-use seal is not closed by witness")
	ErrInputMapMismatch    = errors.New("RGB11 bundle input map does not commit transition input")
	ErrDuplicateStateSpend = errors.New("RGB11 state is spent more than once")
)

type ValidatedState struct {
	Reference schemas.InputRef
	State     schemas.ResolvedInput
	Outpoint  Outpoint
	// SealDisclosure is the strict-encoded disclosed seal that locates this
	// state. Wallets persist it as proof material; indexers must not derive it.
	SealDisclosure []byte
	SealBlinding   uint64
	WitnessTxPtr   bool
	CarrierBinding CarrierBinding
}

// CarrierBinding is derived from the validated DBC proof for the witness that
// created an allocation. It is consensus evidence, not an Indexer projection.
type CarrierBinding struct {
	CommitmentMethod string
	InternalKey      [32]byte
	TapretRoot       [32]byte
	TapretProof      []byte
}

type Validation struct {
	Bundles          int
	Transitions      int
	Anchors          int
	MinedWitnesses   int
	PendingWitnesses int
	CurrentStates    []ValidatedState
	ConsensusValid   bool
}

type validatedOutput struct {
	state          schemas.ResolvedInput
	outpoint       Outpoint
	sealDisclosure []byte
	sealBlinding   uint64
	witnessTxPtr   bool
	carrierBinding CarrierBinding
}

// Validate performs the fail-closed client-side validation required before a
// contract or transfer may affect wallet projection. The resolver supplies
// only Bitcoin facts; all RGB state is derived and checked locally.
func (c *Container) Validate(resolver BitcoinResolver) (Validation, error) {
	report := Validation{}
	if c == nil || !c.StructuralValid || !c.GenesisValid {
		return report, ErrContainerType
	}
	genesis, _ := c.Value.Field("genesis")
	schema, _ := c.Value.Field("schema")
	typeSystem, _ := c.Value.Field("types")
	genesisCommitment, err := operations.CommitGenesis(genesis)
	if err != nil {
		return report, err
	}
	states := make(map[schemas.InputRef]validatedOutput)
	if err := registerOutputs(states, genesisCommitment.OperationID, genesis, [32]byte{}, CarrierBinding{}); err != nil {
		return report, err
	}
	globals, err := contractGlobals(genesis)
	if err != nil {
		return report, err
	}
	context := schemas.TransitionContext{ContractID: genesisCommitment.OperationID, ContractGlobals: globals}
	consumed := make(map[schemas.InputRef]struct{})

	bundlesValue, ok := c.Value.Field("bundles")
	bundlesValue = bundlesValue.Unwrap()
	if !ok || bundlesValue.Kind != strict_types.ValueList {
		return report, operations.ErrInvalidBundle
	}
	for _, witnessBundle := range bundlesValue.Items {
		witnessBundle = witnessBundle.Unwrap()
		publicWitness, witnessOK := witnessBundle.Field("pubWitness")
		anchorValue, anchorOK := witnessBundle.Field("anchor")
		bundleValue, bundleOK := witnessBundle.Field("bundle")
		if !witnessOK || !anchorOK || !bundleOK {
			return report, operations.ErrInvalidBundle
		}
		bundle, err := operations.CommitBundle(bundleValue)
		if err != nil {
			return report, err
		}
		mpcProof, dbcProof, err := decodeAnchor(anchorValue)
		if err != nil {
			return report, err
		}
		mpcCommitment, err := anchors.ConvolveMPC(mpcProof, genesisCommitment.OperationID, bundle.BundleID)
		if err != nil {
			return report, fmt.Errorf("%w: %v", ErrBundleAnchor, err)
		}
		witnessTx, witnessTxID, evidence, err := resolveWitness(publicWitness, resolver)
		if err != nil {
			return report, err
		}
		if err := verifyDBC(witnessTx, dbcProof, mpcCommitment); err != nil {
			return report, err
		}
		carrierBinding, err := decodeCarrierBinding(dbcProof, mpcCommitment)
		if err != nil {
			return report, err
		}
		report.Bundles++
		report.Anchors++
		if evidence.State == WitnessMined {
			report.MinedWitnesses++
		} else {
			report.PendingWitnesses++
		}

		inputMap, _ := bundleValue.Field("inputMap")
		for _, transition := range bundle.Transitions {
			commitment, err := operations.CommitTransition(transition)
			if err != nil {
				return report, err
			}
			inputs, err := transitionInputs(transition)
			if err != nil {
				return report, err
			}
			for _, input := range inputs {
				if !inputMapCommits(inputMap, input, commitment.OperationID) {
					return report, ErrInputMapMismatch
				}
				if _, ok := consumed[input]; ok {
					return report, ErrDuplicateStateSpend
				}
				previous, ok := states[input]
				if !ok {
					return report, schemas.ErrInputUnresolved
				}
				if !txSpends(witnessTx, previous.outpoint) {
					return report, ErrSealNotClosed
				}
				if err := verifySpentOutpoint(resolver, previous.outpoint, witnessTxID); err != nil {
					return report, err
				}
			}
			validation, err := schemas.ValidateTransition(schema, typeSystem, transition, context,
				schemas.InputResolverFunc(func(ref schemas.InputRef) (schemas.ResolvedInput, error) {
					output, ok := states[ref]
					if !ok {
						return schemas.ResolvedInput{}, schemas.ErrInputUnresolved
					}
					return output.state, nil
				}))
			if err != nil {
				return report, err
			}
			for _, input := range inputs {
				consumed[input] = struct{}{}
				delete(states, input)
			}
			if err := registerOutputs(states, validation.OperationID, transition, witnessTxID, carrierBinding); err != nil {
				return report, err
			}
			if err := appendOperationGlobals(context.ContractGlobals, transition); err != nil {
				return report, err
			}
			report.Transitions++
		}
	}

	for reference, output := range states {
		if err := verifyCurrentOutpoint(resolver, output.outpoint); err != nil {
			return report, err
		}
		report.CurrentStates = append(report.CurrentStates, ValidatedState{
			Reference: reference, State: output.state, Outpoint: output.outpoint,
			SealDisclosure: append([]byte(nil), output.sealDisclosure...), SealBlinding: output.sealBlinding,
			WitnessTxPtr: output.witnessTxPtr, CarrierBinding: cloneCarrierBinding(output.carrierBinding),
		})
	}
	report.ConsensusValid = true
	c.ConsensusValid = true
	return report, nil
}

func registerOutputs(states map[schemas.InputRef]validatedOutput, operationID [32]byte, operation strict_types.Value,
	witnessTxID [32]byte, carrierBinding CarrierBinding) error {
	outputs, err := schemas.RevealedOutputs(operation)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		outpoint, blinding, witnessTxPtr, err := assignmentOutpoint(output.Seal, witnessTxID)
		if err != nil {
			return err
		}
		disclosure, err := canonicalSealDisclosure(outpoint, blinding, witnessTxPtr)
		if err != nil {
			return err
		}
		ref := schemas.InputRef{OperationID: operationID, AssignmentType: output.AssignmentType, Index: output.Index}
		states[ref] = validatedOutput{
			state: output.State, outpoint: outpoint,
			sealDisclosure: disclosure,
			sealBlinding:   blinding, witnessTxPtr: witnessTxPtr,
			carrierBinding: cloneCarrierBinding(carrierBinding),
		}
	}
	return nil
}

func canonicalSealDisclosure(outpoint Outpoint, blinding uint64, witnessTxPtr bool) ([]byte, error) {
	if witnessTxPtr {
		return seals.NewWitnessBlindSeal(outpoint.Vout, blinding).StrictBytes()
	}
	seal, err := seals.NewGraphBlindSeal(outpoint.TxID[:], outpoint.Vout, blinding)
	if err != nil {
		return nil, err
	}
	return seal.StrictBytes()
}

func cloneCarrierBinding(binding CarrierBinding) CarrierBinding {
	binding.TapretProof = append([]byte(nil), binding.TapretProof...)
	return binding
}

func assignmentOutpoint(value strict_types.Value, witnessTxID [32]byte) (Outpoint, uint64, bool, error) {
	value = value.Unwrap()
	txidValue, txidOK := value.Field("txid")
	voutValue, voutOK := value.Field("vout")
	blindingValue, blindingOK := value.Field("blinding")
	vout, voutNumberOK := unsignedNumber(voutValue)
	blinding, blindingNumberOK := unsignedNumber(blindingValue)
	if !txidOK || !voutOK || !blindingOK || !voutNumberOK || vout > math.MaxUint32 || !blindingNumberOK {
		return Outpoint{}, 0, false, ErrSealNotClosed
	}
	var txid [32]byte
	witnessTxPtr := false
	if raw, ok := txidValue.Bytes(); ok {
		if len(raw) != 32 {
			return Outpoint{}, 0, false, ErrSealNotClosed
		}
		copy(txid[:], raw)
	} else {
		pointer := txidValue.Unwrap()
		if pointer.Kind != strict_types.ValueUnion {
			return Outpoint{}, 0, false, ErrSealNotClosed
		}
		switch pointer.Name {
		case "witnessTx":
			if witnessTxID == ([32]byte{}) {
				return Outpoint{}, 0, false, ErrSealNotClosed
			}
			txid = witnessTxID
			witnessTxPtr = true
		case "txid":
			if pointer.Inner == nil {
				return Outpoint{}, 0, false, ErrSealNotClosed
			}
			raw, ok := pointer.Inner.Bytes()
			if !ok || len(raw) != 32 {
				return Outpoint{}, 0, false, ErrSealNotClosed
			}
			copy(txid[:], raw)
		default:
			return Outpoint{}, 0, false, ErrSealNotClosed
		}
	}
	return Outpoint{TxID: txid, Vout: uint32(vout)}, blinding, witnessTxPtr, nil
}

func transitionInputs(transition strict_types.Value) ([]schemas.InputRef, error) {
	value, ok := transition.Field("inputs")
	value = value.Unwrap()
	if !ok || (value.Kind != strict_types.ValueList && value.Kind != strict_types.ValueSet) {
		return nil, operations.ErrInvalidTransition
	}
	inputs := make([]schemas.InputRef, 0, len(value.Items))
	for _, item := range value.Items {
		item = item.Unwrap()
		opValue, opOK := item.Field("op")
		typeValue, typeOK := item.Field("ty")
		indexValue, indexOK := item.Field("no")
		op, bytesOK := opValue.Bytes()
		assignmentType, typeNumberOK := unsignedNumber(typeValue)
		index, indexNumberOK := unsignedNumber(indexValue)
		if !opOK || !typeOK || !indexOK || !bytesOK || len(op) != 32 || !typeNumberOK || !indexNumberOK || index > math.MaxUint16 {
			return nil, operations.ErrInvalidTransition
		}
		var operationID [32]byte
		copy(operationID[:], op)
		inputs = append(inputs, schemas.InputRef{OperationID: operationID, AssignmentType: assignmentType, Index: uint16(index)})
	}
	return inputs, nil
}

func inputMapCommits(value strict_types.Value, input schemas.InputRef, operationID [32]byte) bool {
	value = value.Unwrap()
	for _, entry := range value.Entries {
		key := entry.Key.Unwrap()
		opValue, opOK := key.Field("op")
		typeValue, typeOK := key.Field("ty")
		indexValue, indexOK := key.Field("no")
		op, bytesOK := opValue.Bytes()
		assignmentType, typeNumberOK := unsignedNumber(typeValue)
		index, indexNumberOK := unsignedNumber(indexValue)
		mapped, mappedOK := entry.Value.Bytes()
		if opOK && typeOK && indexOK && bytesOK && typeNumberOK && indexNumberOK && mappedOK &&
			len(op) == 32 && len(mapped) == 32 && bytes.Equal(op, input.OperationID[:]) &&
			assignmentType == input.AssignmentType && index == uint64(input.Index) && bytes.Equal(mapped, operationID[:]) {
			return true
		}
	}
	return false
}

func contractGlobals(genesis strict_types.Value) (map[uint64][][]byte, error) {
	value, ok := genesis.Field("globals")
	value = value.Unwrap()
	if !ok || value.Kind != strict_types.ValueMap {
		return nil, operations.ErrInvalidGenesis
	}
	result := make(map[uint64][][]byte)
	for _, entry := range value.Entries {
		id, ok := unsignedNumber(entry.Key)
		states := entry.Value.Unwrap()
		if !ok || states.Kind != strict_types.ValueList {
			return nil, operations.ErrInvalidGenesis
		}
		for _, state := range states.Items {
			raw, ok := state.Bytes()
			if !ok {
				return nil, operations.ErrInvalidGenesis
			}
			result[id] = append(result[id], raw)
		}
	}
	return result, nil
}

// appendOperationGlobals advances the contract-wide global-state view after a
// validated transition. This keeps later transitions on the same history from
// evaluating against genesis-only context.
func appendOperationGlobals(target map[uint64][][]byte, operation strict_types.Value) error {
	updates, err := contractGlobals(operation)
	if err != nil {
		return err
	}
	for typeID, values := range updates {
		for _, value := range values {
			target[typeID] = append(target[typeID], append([]byte(nil), value...))
		}
	}
	return nil
}

func decodeAnchor(value strict_types.Value) (anchors.MPCProof, strict_types.Value, error) {
	value = value.Unwrap()
	mpcValue, mpcOK := value.Field("mpcProof")
	dbcValue, dbcOK := value.Field("dbcProof")
	posValue, posOK := mpcValue.Unwrap().Field("pos")
	cofactorValue, cofactorOK := mpcValue.Unwrap().Field("cofactor")
	pathValue, pathOK := mpcValue.Unwrap().Field("path")
	pos, posNumberOK := unsignedNumber(posValue)
	cofactor, cofactorNumberOK := unsignedNumber(cofactorValue)
	pathValue = pathValue.Unwrap()
	if !mpcOK || !dbcOK || !posOK || !cofactorOK || !pathOK || !posNumberOK || pos > math.MaxUint32 ||
		!cofactorNumberOK || cofactor > math.MaxUint16 || pathValue.Kind != strict_types.ValueList {
		return anchors.MPCProof{}, strict_types.Value{}, ErrBundleAnchor
	}
	proof := anchors.MPCProof{Position: uint32(pos), Cofactor: uint16(cofactor), Path: make([][32]byte, 0, len(pathValue.Items))}
	for _, item := range pathValue.Items {
		raw, ok := item.Bytes()
		if !ok || len(raw) != 32 {
			return anchors.MPCProof{}, strict_types.Value{}, ErrBundleAnchor
		}
		var hash [32]byte
		copy(hash[:], raw)
		proof.Path = append(proof.Path, hash)
	}
	return proof, dbcValue.Unwrap(), nil
}

func verifyDBC(tx *wire.MsgTx, proof strict_types.Value, commitment [32]byte) error {
	if proof.Kind != strict_types.ValueUnion {
		return ErrBundleAnchor
	}
	carrier := firstCarrier(tx)
	if carrier == nil {
		return ErrBundleAnchor
	}
	switch proof.Name {
	case "opret":
		if err := anchors.VerifyOpretScript(carrier.PkScript, commitment); err != nil {
			return fmt.Errorf("%w: %v", ErrBundleAnchor, err)
		}
	case "tapret":
		if proof.Inner == nil || len(carrier.PkScript) != 34 || carrier.PkScript[0] != 0x51 || carrier.PkScript[1] != 0x20 {
			return ErrBundleAnchor
		}
		internal, nonce, partner, err := decodeTapretProof(proof.Inner.Unwrap())
		if err != nil {
			return err
		}
		outputKey, err := anchors.TapretOutputKeyWithPartner(internal, commitment, nonce, partner)
		if err != nil || !bytes.Equal(carrier.PkScript[2:], outputKey[:]) {
			return ErrBundleAnchor
		}
	default:
		return ErrBundleAnchor
	}
	return nil
}

func decodeCarrierBinding(proof strict_types.Value, commitment [32]byte) (CarrierBinding, error) {
	if proof.Kind != strict_types.ValueUnion {
		return CarrierBinding{}, ErrBundleAnchor
	}
	switch proof.Name {
	case "opret":
		return CarrierBinding{CommitmentMethod: "opret1st"}, nil
	case "tapret":
		if proof.Inner == nil {
			return CarrierBinding{}, ErrBundleAnchor
		}
		internal, nonce, partner, err := decodeTapretProof(proof.Inner.Unwrap())
		if err != nil {
			return CarrierBinding{}, err
		}
		root, err := anchors.TapretMerkleRoot(commitment, nonce, partner)
		if err != nil {
			return CarrierBinding{}, err
		}
		return CarrierBinding{
			CommitmentMethod: "tapret1st", InternalKey: internal, TapretRoot: root,
			TapretProof: append([]byte(nil), proof.Encoded...),
		}, nil
	default:
		return CarrierBinding{}, ErrBundleAnchor
	}
}

func firstCarrier(tx *wire.MsgTx) *wire.TxOut {
	for _, output := range tx.TxOut {
		if (len(output.PkScript) > 0 && output.PkScript[0] == 0x6a) ||
			(len(output.PkScript) == 34 && output.PkScript[0] == 0x51 && output.PkScript[1] == 0x20) {
			return output
		}
	}
	return nil
}

func decodeTapretProof(value strict_types.Value) ([32]byte, uint8, *anchors.TapretPartner, error) {
	pathValue, pathOK := value.Field("pathProof")
	internalValue, internalOK := value.Field("internalPk")
	internalRaw, internalBytesOK := internalValue.Bytes()
	partnerValue, partnerOK := pathValue.Unwrap().Field("partnerNode")
	nonceValue, nonceOK := pathValue.Unwrap().Field("nonce")
	nonce, nonceNumberOK := unsignedNumber(nonceValue)
	if !pathOK || !internalOK || !internalBytesOK || len(internalRaw) != 32 || !partnerOK || !nonceOK || !nonceNumberOK || nonce > math.MaxUint8 {
		return [32]byte{}, 0, nil, ErrBundleAnchor
	}
	var internal [32]byte
	copy(internal[:], internalRaw)
	partnerValue = partnerValue.Unwrap()
	if partnerValue.Kind != strict_types.ValueUnion {
		return [32]byte{}, 0, nil, ErrBundleAnchor
	}
	if partnerValue.Name == "none" {
		return internal, uint8(nonce), nil, nil
	}
	if partnerValue.Name != "some" || partnerValue.Inner == nil {
		return [32]byte{}, 0, nil, ErrBundleAnchor
	}
	partnerUnion := partnerValue.Inner.Unwrap()
	if partnerUnion.Kind != strict_types.ValueUnion || partnerUnion.Inner == nil {
		return [32]byte{}, 0, nil, ErrBundleAnchor
	}
	partner := new(anchors.TapretPartner)
	switch partnerUnion.Name {
	case "leftNode":
		raw, ok := partnerUnion.Inner.Bytes()
		if !ok || len(raw) != 32 {
			return [32]byte{}, 0, nil, ErrBundleAnchor
		}
		partner.Kind = anchors.TapretLeftNode
		copy(partner.NodeHash[:], raw)
	case "rightLeaf":
		leaf := partnerUnion.Inner.Unwrap()
		versionValue, versionOK := leaf.Field("version")
		scriptValue, scriptOK := leaf.Field("script")
		version, versionNumberOK := unsignedNumber(versionValue)
		script, scriptBytesOK := scriptValue.Bytes()
		if !versionOK || !scriptOK || !versionNumberOK || version > math.MaxUint8 || !scriptBytesOK {
			return [32]byte{}, 0, nil, ErrBundleAnchor
		}
		partner.Kind, partner.LeafVersion, partner.LeafScript = anchors.TapretRightLeaf, uint8(version), script
	case "rightBranch":
		branch := partnerUnion.Inner.Unwrap()
		leftValue, leftOK := branch.Field("leftNodeHash")
		rightValue, rightOK := branch.Field("rightNodeHash")
		left, leftBytesOK := leftValue.Bytes()
		right, rightBytesOK := rightValue.Bytes()
		if !leftOK || !rightOK || !leftBytesOK || !rightBytesOK || len(left) != 32 || len(right) != 32 {
			return [32]byte{}, 0, nil, ErrBundleAnchor
		}
		partner.Kind = anchors.TapretRightBranch
		copy(partner.LeftNodeHash[:], left)
		copy(partner.RightNodeHash[:], right)
	default:
		return [32]byte{}, 0, nil, ErrBundleAnchor
	}
	return internal, uint8(nonce), partner, nil
}

func txSpends(tx *wire.MsgTx, outpoint Outpoint) bool {
	for _, input := range tx.TxIn {
		if [32]byte(input.PreviousOutPoint.Hash) == outpoint.TxID && input.PreviousOutPoint.Index == outpoint.Vout {
			return true
		}
	}
	return false
}

func verifySpentOutpoint(resolver BitcoinResolver, outpoint Outpoint, spendingTxID [32]byte) error {
	if resolver == nil {
		return ErrOutpointUnknown
	}
	evidence, err := resolver.ResolveRGB11Outpoint(outpoint)
	if err != nil || !evidence.Known || !evidence.Exists {
		return ErrOutpointUnknown
	}
	if !evidence.Spent || evidence.SpendingTxID == nil || *evidence.SpendingTxID != spendingTxID {
		return ErrOutpointSpend
	}
	return nil
}

func verifyCurrentOutpoint(resolver BitcoinResolver, outpoint Outpoint) error {
	if resolver == nil {
		return ErrOutpointUnknown
	}
	evidence, err := resolver.ResolveRGB11Outpoint(outpoint)
	if err != nil || !evidence.Known || !evidence.Exists {
		return ErrOutpointUnknown
	}
	if evidence.Spent {
		return ErrOutpointSpend
	}
	return nil
}
