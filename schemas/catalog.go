// Package schemas identifies every standard schema shipped by the frozen
// rgb-schemas 0.11.1-rc.11 release. Consensus validation code must still
// validate the actual schema and operation data; this catalog is the stable
// capability/metadata layer.
package schemas

import (
	"errors"

	"github.com/sat20-labs/rgb11/baid64"
)

type Kind string

const (
	NIA Kind = "NIA"
	IFA Kind = "IFA"
	PFA Kind = "PFA"
	CFA Kind = "CFA"
	UDA Kind = "UDA"
)

type Descriptor struct {
	Kind               Kind
	Name               string
	SchemaID           string
	SourceFile         string
	SourceSHA256       string
	Fungible           bool
	Inflatable         bool
	Permissioned       bool
	DefaultControlMode string
}

var Standard = []Descriptor{
	{NIA, "NonInflatableAsset", "RWhwUfTMpuP2Zfx1~j4nswCANGeJrYOqDcKelaMV4zU#remote-digital-pegasus", "src/nia.rs", "4d5bb666be20e23c945c2bef86b03ea5b8702867c4bbeb084eafe96366127797", true, false, false, "none"},
	{IFA, "InflatableFungibleAsset", "IpjJhFLz3oywYKQxO3KmFgR0Aa415nlTNrNyEFqMZCE#shoe-colombo-mango", "src/ifa.rs", "5f0c0e5d00ba0bee288309e00ec6e9da253dfc41ce0430926bea07edd01bb115", true, true, false, "none"},
	{PFA, "PermissionedFungibleAsset", "YvvvQ4UsHuPQDT3nIQ9mnpsqMbrs5lYZRbyymHVrkY8#famous-process-eagle", "src/pfa.rs", "8081f919159489e3d8d0070ecdeb50c0fc682e043a40715ea6d7eadb9b01637f", true, true, true, "permissioned"},
	{CFA, "CollectibleFungibleAsset", "JgqK5hJX9YBT4osCV7VcW_iLTcA5csUCnLzvaKTTrNY#mars-house-friend", "src/cfa.rs", "d3fe4a11f11ec689ee0130a55388b7953da5b78be51ba0dd199cdacd9b565ce2", true, false, false, "none"},
	{UDA, "UniqueDigitalAsset", "~6rjymf3GTE840lb5JoXm2aFwE8eWCk3mCjOf_mUztE#spider-montana-fantasy", "src/uda.rs", "5aacded60984aa541ea4ae67bef8483677f242e2dba38a762456fbcadff4e815", false, false, false, "none"},
}

var ErrUnknownSchema = errors.New("unknown RGB11 standard schema")

func ByID(schemaID string) (Descriptor, error) {
	want, err := baid64.Decode32(schemaID, baid64.SchemaIDOptions())
	if err != nil {
		return Descriptor{}, err
	}
	for _, descriptor := range Standard {
		id, err := baid64.Decode32(descriptor.SchemaID, baid64.SchemaIDOptions())
		if err == nil && id == want {
			return descriptor, nil
		}
	}
	return Descriptor{}, ErrUnknownSchema
}

func ByKind(kind Kind) (Descriptor, error) {
	for _, descriptor := range Standard {
		if descriptor.Kind == kind {
			return descriptor, nil
		}
	}
	return Descriptor{}, ErrUnknownSchema
}
