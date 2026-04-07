package modeluuid

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	tsChars  = 9
	rHiChars = 11
	rLoChars = 3
	textLen  = tsChars + rHiChars + rLoChars
)

var base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var decMap [256]int8

func init() {
	for i := range decMap {
		decMap[i] = -1
	}
	for i, c := range []byte(base62Alphabet) {
		decMap[c] = int8(i)
	}
}

func encodeUint64Width(buf []byte, n uint64) {
	i := len(buf) - 1
	for n > 0 {
		r := n % 62
		n /= 62
		buf[i] = base62Alphabet[r]
		i--
	}
	for i >= 0 {
		buf[i] = '0'
		i--
	}
	if n != 0 {
		panic("encodeUint64Width: value too large for field width")
	}
}

func decodeUint64(bytes []byte) (uint64, error) {
	var n uint64
	for _, b := range bytes {
		v := decMap[b]
		if v < 0 {
			return 0, fmt.Errorf("invalid base62 char %q", b)
		}
		n = n*62 + uint64(v)
	}
	return n, nil
}

func EncodeSplit2(u UUID) string {
	var out [textLen]byte

	var tsBuf [8]byte
	copy(tsBuf[2:], u[0:6])
	ts := binary.BigEndian.Uint64(tsBuf[:]) & 0xFFFFFFFFFFFF

	rHi := binary.BigEndian.Uint64(u[6:14])
	rLo := binary.BigEndian.Uint16(u[14:16]) & 0xFFFF

	encodeUint64Width(out[0:tsChars], ts)
	encodeUint64Width(out[tsChars:tsChars+rHiChars], rHi)
	encodeUint64Width(out[tsChars+rHiChars:], uint64(rLo))

	return string(out[:])
}

func DecodeSplit2(s string) (UUID, error) {
	if len(s) != textLen {
		return Nil, errors.New("invalid compact UUID length")
	}
	ts, err := decodeUint64([]byte(s[:tsChars]))
	if err != nil {
		return Nil, err
	}
	rHi, err := decodeUint64([]byte(s[tsChars : tsChars+rHiChars]))
	if err != nil {
		return Nil, err
	}
	rLo, err := decodeUint64([]byte(s[tsChars+rHiChars:]))
	if err != nil {
		return Nil, err
	}

	var u UUID
	binary.BigEndian.PutUint64(u[0:8], ts<<16)
	binary.BigEndian.PutUint64(u[6:14], rHi)
	binary.BigEndian.PutUint16(u[14:16], uint16(rLo))
	return u, nil
}
