package modeluuid

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type UUID [16]byte

var Nil = UUID{}

func New() UUID {
	var out UUID
	ts := uint64(time.Now().UnixMilli()) & 0xFFFFFFFFFFFF
	out[0] = byte(ts >> 40)
	out[1] = byte(ts >> 32)
	out[2] = byte(ts >> 24)
	out[3] = byte(ts >> 16)
	out[4] = byte(ts >> 8)
	out[5] = byte(ts)
	if _, err := rand.Read(out[6:]); err != nil {
		panic(fmt.Sprintf("uuid: rand failed: %v", err))
	}
	return out
}

func (u UUID) Value() (driver.Value, error) {
	if u.IsNull() {
		return nil, nil
	}
	return u[:], nil
}

func (u *UUID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = Nil
		return nil
	case []byte:
		if len(v) != 16 {
			return fmt.Errorf("uuid: invalid byte length %d", len(v))
		}
		copy(u[:], v)
		return nil
	case string:
		id, err := Parse(v)
		if err != nil {
			return err
		}
		*u = id
		return nil
	default:
		return fmt.Errorf("uuid: cannot scan type %T", src)
	}
}

func (UUID) GormDataType() string { return "uuid" }

func (UUID) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "BLOB"
	case "postgres":
		return "UUID"
	default:
		return "BINARY(16)"
	}
}

func (u UUID) IsNull() bool { return u == UUID{} }

func (u UUID) String() string { return EncodeSplit2(u) }

func (u *UUID) ParseString(s string) error {
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	copy(u[:], parsed[:])
	return nil
}

func Parse(s string) (UUID, error) {
	if len(s) == textLen {
		if id, err := DecodeSplit2(s); err == nil {
			return id, nil
		}
	}
	hexText := strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if len(hexText) != 32 {
		return Nil, errors.New("uuid: parse failed")
	}
	body, err := hex.DecodeString(hexText)
	if err != nil {
		return Nil, errors.New("uuid: parse failed")
	}
	var out UUID
	copy(out[:], body)
	return out, nil
}

func (u UUID) MarshalText() ([]byte, error) { return []byte(EncodeSplit2(u)), nil }

func (u *UUID) UnmarshalText(b []byte) error {
	id, err := DecodeSplit2(string(b))
	if err != nil {
		return err
	}
	*u = id
	return nil
}

func (u UUID) MarshalJSON() ([]byte, error) { return json.Marshal(EncodeSplit2(u)) }

func (u *UUID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	id, err := Parse(s)
	if err != nil {
		return err
	}
	*u = id
	return nil
}
