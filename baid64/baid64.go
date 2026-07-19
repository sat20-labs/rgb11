// Package baid64 implements the identity encoding used by the frozen RGB11
// dependency set (baid64 0.4.1).
package baid64

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Upstream-Repository: UBIDECO/rust-baid64
// Upstream-Version: 0.4.1
// Upstream-Commit: 7e6a7c36013b30df597c85e6c3f3464d928e4563
// Upstream-File: src/lib.rs
// Upstream-File-SHA256: 0bfe441a11090781bbbb84c00a77f894ba9f09678198eb1165f12068834b306f
// Translation-Revision: 1

const Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_~"

var (
	ErrInvalidHRI      = errors.New("invalid BAID64 human-readable identifier")
	ErrInvalidLength   = errors.New("invalid BAID64 payload length")
	ErrInvalidChecksum = errors.New("invalid BAID64 checksum")
	ErrInvalidMnemonic = errors.New("invalid BAID64 mnemonic checksum")
)

var encoding = base64.NewEncoding(Alphabet).WithPadding(base64.NoPadding)

type Options struct {
	HRI           string
	Prefix        bool
	Chunking      bool
	ChunkFirst    int
	ChunkLength   int
	EmbedChecksum bool
	Mnemonic      bool
}

func RGBContractOptions() Options {
	return Options{HRI: "rgb", Prefix: true, Chunking: true, ChunkFirst: 8, ChunkLength: 7}
}

func UTXOBlindOptions() Options {
	return Options{
		HRI: "utxob", Prefix: true, Chunking: true, ChunkFirst: 8, ChunkLength: 7,
		EmbedChecksum: true,
	}
}

func SchemaIDOptions() Options {
	return Options{HRI: "rgb:sch", Prefix: true, Mnemonic: true}
}

func ConsignmentIDOptions() Options {
	return Options{
		HRI: "rgb:csg", Prefix: true, Chunking: true, ChunkFirst: 8, ChunkLength: 7,
		Mnemonic: true,
	}
}

func WitnessVoutOptions() Options {
	return Options{
		HRI: "wvout", Prefix: true, Chunking: true, ChunkFirst: 8, ChunkLength: 7,
		EmbedChecksum: true,
	}
}

func checksum(hri string, payload []byte) [4]byte {
	key := sha256.Sum256([]byte(hri))
	h := sha256.New()
	_, _ = h.Write(key[:])
	_, _ = h.Write(payload)
	sum := h.Sum(nil)
	return [4]byte{sum[0], sum[1], sum[1], sum[2]}
}

func Encode32(payload [32]byte, opts Options) (string, error) {
	return Encode(payload[:], opts)
}

// Encode supports the fixed-size BAID64 payloads used by RGB. The checksum is
// calculated over the exact payload length; RGB IDs use 32 bytes and witness
// vout beneficiaries use 33 bytes.
func Encode(payload []byte, opts Options) (string, error) {
	if opts.HRI == "" || len(opts.HRI) > 16 {
		return "", ErrInvalidHRI
	}
	if len(payload) == 0 {
		return "", ErrInvalidLength
	}
	data := append([]byte{}, payload...)
	if opts.EmbedChecksum {
		ck := checksum(opts.HRI, payload)
		data = append(data, ck[:]...)
	}
	encoded := encoding.EncodeToString(data)
	if opts.Chunking {
		first, size := opts.ChunkFirst, opts.ChunkLength
		if first <= 0 {
			first = 8
		}
		if size <= 0 {
			size = 7
		}
		if len(encoded) > first {
			var b strings.Builder
			b.WriteString(encoded[:first])
			for offset := first; offset < len(encoded); offset += size {
				end := min(offset+size, len(encoded))
				b.WriteByte('-')
				b.WriteString(encoded[offset:end])
			}
			encoded = b.String()
		}
	}
	if opts.Prefix {
		encoded = opts.HRI + ":" + encoded
	}
	if opts.Mnemonic {
		ck := checksum(opts.HRI, payload)
		encoded += "#" + encodeMnemonic4(ck)
	}
	return encoded, nil
}

func Decode32(value string, opts Options) ([32]byte, error) {
	payload, err := Decode(value, 32, opts)
	if err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], payload)
	return result, nil
}

// Decode validates HRI, embedded and mnemonic checksums and returns exactly
// expectedLen payload bytes.
func Decode(value string, expectedLen int, opts Options) ([]byte, error) {
	original := value
	var suffixChecksum *[4]byte
	if i := strings.IndexByte(value, '#'); i >= 0 {
		decoded, err := decodeMnemonic4(value[i+1:])
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidMnemonic, original)
		}
		suffixChecksum = &decoded
		value = value[:i]
	}
	if i := strings.LastIndexByte(value, ':'); i >= 0 {
		if value[:i] != opts.HRI {
			return nil, fmt.Errorf("%w: %q", ErrInvalidHRI, original)
		}
		value = value[i+1:]
	}
	value = strings.ReplaceAll(value, "-", "")
	data, err := encoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode BAID64: %w", err)
	}
	if len(data) != expectedLen && len(data) != expectedLen+4 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidLength, len(data))
	}
	payload := append([]byte{}, data[:expectedLen]...)
	// The upstream parser gives an embedded checksum precedence over a
	// mnemonic suffix when both are present.
	if len(data) == expectedLen+4 {
		embedded := [4]byte{data[expectedLen], data[expectedLen+1], data[expectedLen+2], data[expectedLen+3]}
		suffixChecksum = &embedded
	}
	if suffixChecksum != nil {
		want := checksum(opts.HRI, payload)
		if *suffixChecksum != want {
			return nil, ErrInvalidChecksum
		}
	}
	return payload, nil
}

const mnemonicBase = uint32(1626)

func encodeMnemonic4(value [4]byte) string {
	x := binary.LittleEndian.Uint32(value[:])
	words := [3]string{
		mnemonicWords[x%mnemonicBase],
		mnemonicWords[(x/mnemonicBase)%mnemonicBase],
		mnemonicWords[(x/mnemonicBase/mnemonicBase)%mnemonicBase],
	}
	return strings.Join(words[:], "-")
}

func decodeMnemonic4(value string) ([4]byte, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r > 0x7f || !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	})
	if len(parts) != 3 {
		return [4]byte{}, ErrInvalidMnemonic
	}
	indices := make(map[string]uint32, len(mnemonicWords))
	// The upstream HashMap keeps the last index of duplicate words.
	for index, word := range mnemonicWords {
		indices[word] = uint32(index)
	}
	first, ok := indices[parts[0]]
	if !ok || first >= mnemonicBase {
		return [4]byte{}, ErrInvalidMnemonic
	}
	second, ok := indices[parts[1]]
	if !ok || second >= mnemonicBase {
		return [4]byte{}, ErrInvalidMnemonic
	}
	third, ok := indices[parts[2]]
	if !ok || third >= mnemonicBase || third >= 1625 || (third == 1624 && first+second*mnemonicBase > 1312671) {
		return [4]byte{}, ErrInvalidMnemonic
	}
	x := first + second*mnemonicBase + third*mnemonicBase*mnemonicBase
	var out [4]byte
	binary.LittleEndian.PutUint32(out[:], x)
	return out, nil
}
