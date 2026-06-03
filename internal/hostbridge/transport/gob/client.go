package gobtransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

// ConnTransport sends one opaque payload over one freshly-dialed connection.
// The server closes the connection after writing the response; that close frames
// the response for io.ReadAll.
type ConnTransport struct {
	Address string
	Dialer  transport.Dialer
}

func (t *ConnTransport) Send(ctx context.Context, payload []byte) ([]byte, error) {
	if t == nil {
		return nil, fmt.Errorf("missing gob connection transport")
	}
	if t.Dialer == nil {
		return nil, fmt.Errorf("missing gob connection dialer")
	}
	conn, err := t.Dialer.Dial(ctx, t.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := io.Copy(conn, bytes.NewReader(payload)); err != nil {
		return nil, fmt.Errorf("write command request: %w", err)
	}
	resp, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("read command response: %w", err)
	}
	return resp, nil
}

// CommandRunner encodes hostbridge commands with gob over a byte transport.
type CommandRunner struct {
	Transport transport.ByteTransport
}

func NewCommandRunner(address string, tlsConfig *tls.Config) *CommandRunner {
	return &CommandRunner{
		Transport: &ConnTransport{
			Address: address,
			Dialer:  &Dialer{TLSConfig: tlsConfig},
		},
	}
}

func (r *CommandRunner) RunCommand(ctx context.Context, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error) {
	if r == nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("missing gob command runner")
	}
	if r.Transport == nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("missing gob command transport")
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(req); err != nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("encode command request: %w", err)
	}
	respPayload, err := r.Transport.Send(ctx, payload.Bytes())
	if err != nil {
		return hostbridge.CommandResponse{}, err
	}
	var resp hostbridge.CommandResponse
	if err := gob.NewDecoder(bytes.NewReader(respPayload)).Decode(&resp); err != nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("decode command response: %w", err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
