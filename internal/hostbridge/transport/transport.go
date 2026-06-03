package transport

import (
	"context"
	"net"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

// ByteTransport moves one opaque request payload and returns one opaque response payload.
// It owns wire concerns only; command encoding and command handling live above it.
type ByteTransport interface {
	Send(ctx context.Context, payload []byte) ([]byte, error)
}

// CommandRunner executes one typed hostbridge command over an underlying transport.
// Implementations own the command codec, such as gob or JSON.
type CommandRunner interface {
	RunCommand(ctx context.Context, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error)
}

// Dialer opens a connection for connection-oriented byte transports.
// Implementations own connection setup concerns such as TLS material loading.
type Dialer interface {
	Dial(ctx context.Context, address string) (net.Conn, error)
}
