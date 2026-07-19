// Package consignment handles the transport envelope around RGB11 strict
// consignments. Successful armor parsing proves transport integrity only; it
// does not imply RGB consensus validity.
package consignment

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sat20-labs/rgb11/baid64"
)

// Upstream-Repository: rgb-protocol/rgb-ops
// Upstream-Version: 0.11.1-rc.11
// Upstream-Commit: 5308b9d46c91857513ff5be2459992264687632b
// Upstream-File: src/containers/consignment.rs
// Upstream-File-SHA256: 553db5473f1e4b87d2a72da130c8ad5f37f72ea1229f2524f9fd0ef98594be3b
// Translation-Revision: 1

const (
	plateTitle         = "RGB CONSIGNMENT"
	armorBegin         = "-----BEGIN " + plateTitle + "-----"
	armorEnd           = "-----END " + plateTitle + "-----"
	maxArmoredDataSize = 0xFFFFFF
)

var (
	ErrArmorStructure  = errors.New("invalid RGB consignment armor structure")
	ErrArmorHeader     = errors.New("invalid RGB consignment armor header")
	ErrArmorChecksum   = errors.New("RGB consignment armor checksum mismatch")
	ErrArmorTooLarge   = errors.New("RGB consignment exceeds strict U24 confinement")
	ErrArmorIdentifier = errors.New("invalid RGB consignment identifier")
)

type Armor struct {
	ID       string
	Version  uint32
	Type     string
	Contract string
	Schema   string
	Headers  map[string]string
	Data     []byte
	SHA256   [32]byte
}

func ParseArmor(text string) (*Armor, error) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	// Allow the official maximum encoded body plus headers. Scanner's default
	// token limit is too small for a valid single-line armor.
	scanner.Buffer(make([]byte, 4096), maxArmoredDataSize*2)
	foundBegin := false
	readingHeaders := false
	readingBody := false
	foundEnd := false
	headers := make(map[string]string)
	var body strings.Builder
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if !foundBegin {
			if line == armorBegin {
				foundBegin = true
				readingHeaders = true
			}
			continue
		}
		if readingHeaders {
			if line == "" {
				readingHeaders = false
				readingBody = true
				continue
			}
			name, value, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("%w: %q", ErrArmorHeader, line)
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if _, duplicate := headers[name]; duplicate {
				return nil, fmt.Errorf("%w: duplicate %s", ErrArmorHeader, name)
			}
			headers[name] = value
			continue
		}
		if readingBody {
			if line == armorEnd {
				foundEnd = true
				break
			}
			if strings.TrimSpace(line) != "" {
				body.WriteString(strings.TrimSpace(line))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !foundBegin || !foundEnd || readingHeaders || body.Len() == 0 {
		return nil, ErrArmorStructure
	}
	data, err := decodeBase85(body.String())
	if err != nil {
		return nil, err
	}
	if len(data) > maxArmoredDataSize {
		return nil, ErrArmorTooLarge
	}
	digest := sha256.Sum256(data)
	checksum, ok := headers["Check-SHA256"]
	if !ok {
		return nil, fmt.Errorf("%w: missing Check-SHA256", ErrArmorHeader)
	}
	want, err := hex.DecodeString(checksum)
	if err != nil || len(want) != sha256.Size || !bytes.Equal(want, digest[:]) {
		return nil, ErrArmorChecksum
	}
	id := headers["Id"]
	if _, err := baid64.Decode32(id, baid64.ConsignmentIDOptions()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArmorIdentifier, err)
	}
	versionValue, ok := headers["Version"]
	if !ok {
		return nil, fmt.Errorf("%w: missing Version", ErrArmorHeader)
	}
	version, err := strconv.ParseUint(versionValue, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%w: Version: %v", ErrArmorHeader, err)
	}
	result := &Armor{
		ID:       id,
		Version:  uint32(version),
		Type:     headers["Type"],
		Contract: headers["Contract"],
		Schema:   headers["Schema"],
		Headers:  headers,
		Data:     data,
		SHA256:   digest,
	}
	if result.Type != "transfer" && result.Type != "contract" {
		return nil, fmt.Errorf("%w: Type %q", ErrArmorHeader, result.Type)
	}
	return result, nil
}
