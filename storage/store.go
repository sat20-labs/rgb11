package storage

import "errors"

var ErrNotFound = errors.New("RGB11 object not found")

// Tx is the minimal atomic storage contract required by the engine. Wallet SDK
// adapters must update proof sidecars and projected outputs in one transaction.
type Tx interface {
	Put(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	Rollback() error
}

type Store interface {
	Get(key []byte) ([]byte, error)
	Begin() (Tx, error)
}
