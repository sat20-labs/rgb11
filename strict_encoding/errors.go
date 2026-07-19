package strict_encoding

import "errors"

var (
	ErrOutOfBounds      = errors.New("strict encoding value exceeds confinement")
	ErrUnexpectedEOF    = errors.New("unexpected end of strict encoded data")
	ErrTrailingData     = errors.New("trailing strict encoded data")
	ErrInvalidBool      = errors.New("invalid strict encoded boolean")
	ErrInvalidOptionTag = errors.New("invalid strict encoded option tag")
)
