package copilot

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
)

var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func canonicalSessionID(value string) (string, error) {
	if !sessionIDPattern.MatchString(value) || value == "00000000-0000-0000-0000-000000000000" {
		return "", fmt.Errorf("invalid copilot session UUID")
	}
	return strings.ToLower(value), nil
}

func newSessionID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}
