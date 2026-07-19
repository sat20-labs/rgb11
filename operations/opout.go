// Package operations defines RGB11 operation pointers and state transition
// inputs. Full Genesis/Transition validation remains in the consensus
// validator; these types preserve official wire and text representations.
package operations

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/sat20-labs/rgb11/consensus"
	strict "github.com/sat20-labs/rgb11/strict_encoding"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/operation/operations.rs
// Translation-Revision: 1

var (
	ErrInvalidOpout = errors.New("invalid RGB11 operation output")
	ErrInputBounds  = errors.New("RGB11 operation inputs exceed confinement")
)

type AssignmentType uint16

const AssetAssignment AssignmentType = 4000

type Opout struct {
	Operation consensus.OperationID
	Type      AssignmentType
	Number    uint16
}

func ParseOpout(value string) (Opout, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return Opout{}, ErrInvalidOpout
	}
	operationBytes, err := hex.DecodeString(parts[0])
	if err != nil || len(operationBytes) != 32 {
		return Opout{}, ErrInvalidOpout
	}
	id, _ := consensus.IDFromBytes(operationBytes)
	typeID, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return Opout{}, ErrInvalidOpout
	}
	number, err := strconv.ParseUint(parts[2], 10, 16)
	if err != nil {
		return Opout{}, ErrInvalidOpout
	}
	return Opout{Operation: consensus.OperationID(id), Type: AssignmentType(typeID), Number: uint16(number)}, nil
}

func (o Opout) String() string {
	return o.Operation.Hex() + "/" + strconv.FormatUint(uint64(o.Type), 10) + "/" + strconv.FormatUint(uint64(o.Number), 10)
}

func (o Opout) StrictEncode(w io.Writer) error {
	e := strict.NewEncoder(w)
	operation := consensus.ID(o.Operation).Bytes()
	if err := e.Raw(operation[:]); err != nil {
		return err
	}
	if err := e.U16(uint16(o.Type)); err != nil {
		return err
	}
	return e.U16(o.Number)
}

func (o Opout) StrictBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := o.StrictEncode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type Inputs []Opout

func NewInputs(values ...Opout) (Inputs, error) {
	if len(values) == 0 || len(values) > 0xFFFF {
		return nil, ErrInputBounds
	}
	copyValues := append([]Opout(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].String() < copyValues[j].String() })
	for index := 1; index < len(copyValues); index++ {
		if copyValues[index] == copyValues[index-1] {
			return nil, fmt.Errorf("%w: duplicate %s", ErrInvalidOpout, copyValues[index])
		}
	}
	return Inputs(copyValues), nil
}
