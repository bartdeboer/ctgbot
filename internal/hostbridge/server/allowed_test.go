package server

import (
	"encoding/json"
	"testing"
)

func TestAllowedCommandJSONAcceptsSnakeCaseExtraArgs(t *testing.T) {
	t.Parallel()

	var allowed map[string]AllowedCommand
	if err := json.Unmarshal([]byte(`{
		"docker": {
			"name": "docker",
			"allow_extra_args": true
		}
	}`), &allowed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !allowed["docker"].AllowExtraArgs {
		t.Fatalf("allow_extra_args was not decoded: %#v", allowed["docker"])
	}
}
