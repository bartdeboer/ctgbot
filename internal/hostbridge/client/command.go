package client

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
)

func DoCommand(ctx context.Context, address string, tlsDir string, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error) {
	runner := &gobtransport.CommandRunner{
		Transport: &gobtransport.ConnTransport{
			Address: address,
			Dialer:  &gobtransport.Dialer{TLSDir: tlsDir},
		},
	}
	return runner.RunCommand(ctx, req)
}

func DoCommandConn(conn net.Conn, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error) {
	if conn == nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("missing connection")
	}
	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("encode command request: %w", err)
	}
	var resp hostbridge.CommandResponse
	if err := gob.NewDecoder(conn).Decode(&resp); err != nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("decode command response: %w", err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
