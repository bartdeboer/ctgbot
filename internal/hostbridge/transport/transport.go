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

// StreamEvent is one transport-level event delivered by a long-lived event stream.
// Data is intentionally opaque at this layer; event codecs above the transport
// decide whether it is JSON, gob, text, or another application payload.
type StreamEvent struct {
	ID   string
	Type string
	Data []byte
}

// EventSink receives events from an EventStreamTransport.
type EventSink interface {
	Event(ctx context.Context, event StreamEvent) error
}

// EventStreamTransport subscribes to a long-lived stream of transport events.
// It is an optional capability: transports that only support one request/one
// response implement ByteTransport, while SSE/WebSocket/gob-stream transports
// can implement this interface as well.
type EventStreamTransport interface {
	Subscribe(ctx context.Context, payload []byte, sink EventSink) error
}

// CommandRunner executes one typed hostbridge command over an underlying transport.
// Implementations own the command codec, such as gob or JSON.
type CommandRunner interface {
	RunCommand(ctx context.Context, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error)
}

// PeerIdentity is the transport-derived identity for one command request.
// Local hostbridge v1 uses CommonName to bind a container to its thread.
// Remote controller calls use FingerprintSHA256 to authorize stable instance identities.
type PeerIdentity struct {
	CommonName        string
	FingerprintSHA256 string
	TLS               bool
}

// CommandHandler handles one decoded hostbridge command.
type CommandHandler interface {
	HandleCommand(ctx context.Context, peer PeerIdentity, req hostbridge.CommandRequest) hostbridge.CommandResponse
}

// Dialer opens a connection for connection-oriented byte transports.
// Implementations own connection setup concerns such as TLS material loading.
type Dialer interface {
	Dial(ctx context.Context, address string) (net.Conn, error)
}
