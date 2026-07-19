package consignment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

var ErrBuildConsignment = errors.New("unable to build RGB11 transfer consignment")

// BuildTransfer appends a witness bundle to validated contract history and
// selects the provided concealed seals as the new transfer terminals.
func BuildTransfer(base *Container, witnessBundle strict_types.Value, terminalSecrets [][32]byte) (*Container, error) {
	if base == nil || !base.StructuralValid || len(terminalSecrets) > 0xffff {
		return nil, ErrBuildConsignment
	}
	bundleValue, ok := witnessBundle.Unwrap().Field("bundle")
	if !ok {
		return nil, ErrBuildConsignment
	}
	bundle, err := operations.CommitBundle(bundleValue)
	if err != nil {
		return nil, err
	}
	registry, err := strict_types.RC11Registry()
	if err != nil {
		return nil, err
	}
	bundleID, err := registry.Decode("RGBCommit", "BundleId", bundle.BundleID[:])
	if err != nil {
		return nil, err
	}
	var terminalEntries []strict_types.Entry
	if len(terminalSecrets) > 0 {
		secrets := append([][32]byte(nil), terminalSecrets...)
		sort.Slice(secrets, func(i, j int) bool { return bytes.Compare(secrets[i][:], secrets[j][:]) < 0 })
		for index := 1; index < len(secrets); index++ {
			if secrets[index-1] == secrets[index] {
				return nil, ErrBuildConsignment
			}
		}
		secretBytes := make([]byte, 2+32*len(secrets))
		binary.LittleEndian.PutUint16(secretBytes[:2], uint16(len(secrets)))
		for index, secret := range secrets {
			copy(secretBytes[2+index*32:], secret[:])
		}
		secretSet, err := registry.Decode("RGBStd", "SecretSeals", secretBytes)
		if err != nil {
			return nil, err
		}
		terminalEntries = []strict_types.Entry{{Key: bundleID, Value: secretSet}}
	}

	value := base.Value.Clone()
	transfer := fieldPointer(&value, "transfer")
	terminals := fieldPointer(&value, "terminals")
	bundles := fieldPointer(&value, "bundles")
	if transfer == nil || terminals == nil || bundles == nil {
		return nil, ErrBuildConsignment
	}
	*transfer = strict_types.Value{Kind: strict_types.ValueEnum, Name: "true", Text: "true", Tag: 1}
	terminals = unwrapPointer(terminals)
	if terminals.Kind != strict_types.ValueMap {
		return nil, ErrBuildConsignment
	}
	terminals.Entries = terminalEntries
	bundles = unwrapPointer(bundles)
	if bundles.Kind != strict_types.ValueList || len(bundles.Items) >= int(^uint32(0)) {
		return nil, ErrBuildConsignment
	}
	bundles.Items = append(bundles.Items, witnessBundle.Clone())
	encoded, err := registry.Encode("RGBStd", "Consignmenttrue", value)
	if err != nil {
		return nil, err
	}
	rebuilt, err := registry.Decode("RGBStd", "Consignmenttrue", encoded)
	if err != nil {
		return nil, err
	}
	result := *base
	result.Value = rebuilt
	result.Armor = nil
	result.ConsensusValid = false
	return &result, nil
}
