package anchors

import (
	"errors"
	"strings"
)

// Upstream-Repository: rgb-protocol/rgb-consensus
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 44e79963aa4603270eee9aa112ef07a512345e98
// Upstream-File: src/dbc/proof.rs
// Upstream-File-SHA256: 52d66b04e55c1928acc1d37a9a6b7a561d933283b99b0a9fcff9c616bec99bf1
// Translation-Revision: 1

// CloseMethod follows RGB PSBT close-method naming. rgb-lib 0.3.0-beta.7
// currently constructs OpretFirst by default; both variants remain supported.
type CloseMethod uint8

const (
	OpretFirst CloseMethod = iota
	TapretFirst
)

var ErrUnknownCloseMethod = errors.New("unknown RGB11 close method")

func (m CloseMethod) String() string {
	switch m {
	case OpretFirst:
		return "opret1st"
	case TapretFirst:
		return "tapret1st"
	default:
		return "unknown"
	}
}

func ParseCloseMethod(value string) (CloseMethod, error) {
	switch strings.ToLower(value) {
	case "opret1st", "opret-first":
		return OpretFirst, nil
	case "tapret1st", "tapret-first":
		return TapretFirst, nil
	default:
		return 0, ErrUnknownCloseMethod
	}
}
