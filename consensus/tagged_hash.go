package consensus

import "crypto/sha256"

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/commit_verify/digest.rs
// Upstream-File-SHA256: 56e4911a170271c7c1cb6305637e3e7391621b9542d1a0fb80e7e3265a5139c9
// Translation-Revision: 1

// TaggedHash applies the SHA256(tag) || SHA256(tag) domain separator used by
// the frozen RGB consensus code.
func TaggedHash(tag string, chunks ...[]byte) [32]byte {
	tagHash := sha256.Sum256([]byte(tag))
	h := sha256.New()
	_, _ = h.Write(tagHash[:])
	_, _ = h.Write(tagHash[:])
	for _, chunk := range chunks {
		_, _ = h.Write(chunk)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
