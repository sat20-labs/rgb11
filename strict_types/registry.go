package strict_types

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Upstream-Repository: rgb-protocol/rgb-strict-types
// Upstream-Version: 1.0.2
// Upstream-Commit: b441bc508a9fcb556e243c7a9c48f42d4582e32e
// Upstream-File: src/typelib/type_lib.rs
// Upstream-File: src/typesys/translate.rs
// Translation-Revision: 1

var (
	ErrLibraryAbsent = errors.New("strict type library is absent")
	ErrTypeAbsent    = errors.New("strict type is absent")
)

type typeLibrary struct {
	ID          string
	Name        string
	bySemID     map[string]json.RawMessage
	byName      map[string]json.RawMessage
	semIDByName map[string]string
}

type Registry struct {
	byID   map[string]*typeLibrary
	byName map[string]*typeLibrary
}

type libraryEnvelope struct {
	ID          string            `json:"id"`
	NamedSemIDs map[string]string `json:"namedSemIds"`
	Library     struct {
		Name  string                     `json:"name"`
		Types map[string]json.RawMessage `json:"types"`
	} `json:"library"`
}

func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]*typeLibrary), byName: make(map[string]*typeLibrary)}
}

func (r *Registry) AddLibrary(data []byte) error {
	if r == nil {
		return ErrLibraryAbsent
	}
	var envelope libraryEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode strict type library: %w", err)
	}
	if len(envelope.ID) != 64 || envelope.Library.Name == "" || len(envelope.Library.Types) == 0 {
		return errors.New("invalid strict type library envelope")
	}
	lib := &typeLibrary{
		ID: envelope.ID, Name: envelope.Library.Name,
		bySemID: make(map[string]json.RawMessage), byName: envelope.Library.Types,
		semIDByName: envelope.NamedSemIDs,
	}
	for name, semID := range envelope.NamedSemIDs {
		typeDef, ok := envelope.Library.Types[name]
		if !ok || len(semID) != 64 {
			return fmt.Errorf("invalid named strict type %s.%s", envelope.Library.Name, name)
		}
		lib.bySemID[semID] = typeDef
	}
	if _, duplicate := r.byID[lib.ID]; duplicate {
		return fmt.Errorf("duplicate strict type library id %s", lib.ID)
	}
	if _, duplicate := r.byName[lib.Name]; duplicate {
		return fmt.Errorf("duplicate strict type library name %s", lib.Name)
	}
	r.byID[lib.ID] = lib
	r.byName[lib.Name] = lib
	return nil
}

func LoadRegistry(source fs.FS) (*Registry, error) {
	registry := NewRegistry()
	entries, err := fs.Glob(source, "*.json")
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrLibraryAbsent
	}
	for _, path := range entries {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, err
		}
		if err := registry.AddLibrary(data); err != nil {
			return nil, fmt.Errorf("load %s: %w", filepath.Base(path), err)
		}
	}
	return registry, nil
}

//go:embed schema/rc11/*.json
var rc11Libraries embed.FS

func RC11Registry() (*Registry, error) {
	root, err := fs.Sub(rc11Libraries, "schema/rc11")
	if err != nil {
		return nil, err
	}
	registry, err := LoadRegistry(root)
	if err != nil {
		return nil, err
	}
	// EAnchor is a generic Anchor<RGBLogic.DbcProof>. The frozen TypeLibs
	// expose its semantic id as an external BPCore type, while bp_core_stl
	// contains only the two concrete Opret/Tapret anchor instantiations. Keep
	// the exact monomorphized definition required by RGBStd consignments.
	const anchorDbcProof = "833a88f44775fae61efcfc06f6876bad027f86eed05a8ed4e8fb89e067ba2687"
	bpCore, err := registry.library("BPCore")
	if err != nil {
		return nil, err
	}
	bpCore.bySemID[anchorDbcProof] = json.RawMessage(`{"Struct":[{"name":"mpcProof","ty":{"extern":{"libId":"b1c6aeb1c15b1eb68477ab69abfaa807098164db6303ee0aaad3459e2431af96","semId":"36a4b4db8009d8790433ae3d1aed61719206e799fdd1abd504867b9d374c076f"}}},{"name":"dbcProof","ty":{"extern":{"libId":"742e975a8ab1582191efc07f3920b778f531ed9e1c6577b845ee7acc9af0683b","semId":"5d81fb469b06c44019333ff09f29b9a3530f8d9aada7ce5265d192193da5c26f"}}}]}`)
	return registry, nil
}

func (r *Registry) library(nameOrID string) (*typeLibrary, error) {
	if lib := r.byName[nameOrID]; lib != nil {
		return lib, nil
	}
	if lib := r.byID[strings.ToLower(nameOrID)]; lib != nil {
		return lib, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrLibraryAbsent, nameOrID)
}

func (lib *typeLibrary) named(name string) (json.RawMessage, error) {
	if typeDef, ok := lib.byName[name]; ok {
		return typeDef, nil
	}
	return nil, fmt.Errorf("%w: %s.%s", ErrTypeAbsent, lib.Name, name)
}

func (lib *typeLibrary) semantic(semID string) (json.RawMessage, error) {
	if typeDef, ok := lib.bySemID[strings.ToLower(semID)]; ok {
		return typeDef, nil
	}
	return nil, fmt.Errorf("%w: %s:%s", ErrTypeAbsent, lib.Name, semID)
}
