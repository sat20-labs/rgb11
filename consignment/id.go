package consignment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/sat20-labs/rgb11/baid64"
	"github.com/sat20-labs/rgb11/consensus"
	"github.com/sat20-labs/rgb11/operations"
	"github.com/sat20-labs/rgb11/strict_types"
)

const (
	consignmentCommitmentTag = "urn:lnp-bp:rgb:consignment#2024-03-11"
	typeSystemIDTag          = "urn:ubideco:strict-types:sys:v01"
	aluVMLibraryIDTag        = "urn:ubideco:aluvm:lib:v01#230304"
)

var ErrConsignmentID = errors.New("RGB11 consignment id mismatch")

func ID(value strict_types.Value) ([32]byte, error) {
	version, versionOK := value.Field("version")
	transfer, transferOK := value.Field("transfer")
	terminals, terminalsOK := value.Field("terminals")
	genesis, genesisOK := value.Field("genesis")
	bundles, bundlesOK := value.Field("bundles")
	types, typesOK := value.Field("types")
	scripts, scriptsOK := value.Field("scripts")
	if !versionOK || !transferOK || !terminalsOK || !genesisOK || !bundlesOK || !typesOK || !scriptsOK {
		return [32]byte{}, ErrConsignmentID
	}
	genesisCommitment, err := operations.CommitGenesis(genesis)
	if err != nil {
		return [32]byte{}, err
	}
	genesisDisclosure, err := operations.DiscloseHash(genesis)
	if err != nil {
		return [32]byte{}, err
	}
	bundles = bundles.Unwrap()
	if bundles.Kind != strict_types.ValueList {
		return [32]byte{}, ErrConsignmentID
	}
	bundleDisclosures := make([][32]byte, 0, len(bundles.Items))
	for _, item := range bundles.Items {
		bundleID := consensus.TaggedHash(operations.DiscloseCommitmentTag, item.Encoded)
		bundleDisclosures = append(bundleDisclosures, bundleID)
	}
	typeSystemID, err := typeSystemID(types)
	if err != nil {
		return [32]byte{}, err
	}
	scriptIDs, err := aluVMLibraryIDs(scripts)
	if err != nil {
		return [32]byte{}, err
	}
	encoded := append(append([]byte(nil), version.Encoded...), transfer.Encoded...)
	encoded = append(encoded, genesisCommitment.OperationID[:]...)
	encoded = append(encoded, genesisDisclosure[:]...)
	var bundleCount [4]byte
	binary.LittleEndian.PutUint32(bundleCount[:], uint32(len(bundleDisclosures)))
	encoded = append(encoded, bundleCount[:]...)
	for _, id := range bundleDisclosures {
		encoded = append(encoded, id[:]...)
	}
	encoded = append(encoded, terminals.Encoded...)
	encoded = append(encoded, typeSystemID[:]...)
	var scriptCount [2]byte
	binary.LittleEndian.PutUint16(scriptCount[:], uint16(len(scriptIDs)))
	encoded = append(encoded, scriptCount[:]...)
	for _, id := range scriptIDs {
		encoded = append(encoded, id[:]...)
	}
	return consensus.TaggedHash(consignmentCommitmentTag, encoded), nil
}

func typeSystemID(value strict_types.Value) ([32]byte, error) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueMap || len(value.Entries) > 0xffffff {
		return [32]byte{}, ErrConsignmentID
	}
	encoded := []byte{byte(len(value.Entries)), byte(len(value.Entries) >> 8), byte(len(value.Entries) >> 16)}
	for _, entry := range value.Entries {
		raw, ok := entry.Key.Bytes()
		if !ok || len(raw) != 32 {
			return [32]byte{}, ErrConsignmentID
		}
		encoded = append(encoded, raw...)
	}
	return consensus.TaggedHash(typeSystemIDTag, encoded), nil
}

func aluVMLibraryIDs(value strict_types.Value) ([][32]byte, error) {
	value = value.Unwrap()
	if value.Kind != strict_types.ValueSet || len(value.Items) > 0xffff {
		return nil, ErrConsignmentID
	}
	ids := make([][32]byte, 0, len(value.Items))
	for _, library := range value.Items {
		isaeValue, isaeOK := library.Unwrap().Field("isae")
		codeValue, codeOK := library.Unwrap().Field("code")
		dataValue, dataOK := library.Unwrap().Field("data")
		libsValue, libsOK := library.Unwrap().Field("libs")
		code, codeBytesOK := codeValue.Bytes()
		data, dataBytesOK := dataValue.Bytes()
		isaeValue = isaeValue.Unwrap()
		libsValue = libsValue.Unwrap()
		if !isaeOK || !codeOK || !dataOK || !libsOK || !codeBytesOK || !dataBytesOK ||
			isaeValue.Kind != strict_types.ValueSet || libsValue.Kind != strict_types.ValueSet || len(libsValue.Items) > 0xff {
			return nil, ErrConsignmentID
		}
		names := make([]string, 0, len(isaeValue.Items))
		for _, item := range isaeValue.Items {
			name, ok := item.TextValue()
			if !ok {
				return nil, ErrConsignmentID
			}
			names = append(names, name)
		}
		encoded := []byte{byte(len(strings.Join(names, " ")))}
		encoded = append(encoded, []byte(strings.Join(names, " "))...)
		if len(code) > 0xffff || len(data) > 0xffff {
			return nil, ErrConsignmentID
		}
		var length [2]byte
		binary.LittleEndian.PutUint16(length[:], uint16(len(code)))
		encoded = append(encoded, length[:]...)
		encoded = append(encoded, code...)
		binary.LittleEndian.PutUint16(length[:], uint16(len(data)))
		encoded = append(encoded, length[:]...)
		encoded = append(encoded, data...)
		encoded = append(encoded, byte(len(libsValue.Items)))
		for _, dependency := range libsValue.Items {
			raw, ok := dependency.Bytes()
			if !ok || len(raw) != 32 {
				return nil, ErrConsignmentID
			}
			encoded = append(encoded, raw...)
		}
		ids = append(ids, consensus.TaggedHash(aluVMLibraryIDTag, encoded))
	}
	return ids, nil
}

func verifyID(header string, value strict_types.Value) error {
	want, err := baid64.Decode32(header, baid64.ConsignmentIDOptions())
	if err != nil {
		return err
	}
	actual, err := ID(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(want[:], actual[:]) {
		return fmt.Errorf("%w: header=%s", ErrConsignmentID, header)
	}
	return nil
}
