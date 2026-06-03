package gobtransport

import (
	"context"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

// CommandHandler handles one decoded command request. clientIdentity is derived
// from the peer certificate when the connection is TLS.
type CommandHandler interface {
	HandleCommand(ctx context.Context, clientIdentity string, req hostbridge.CommandRequest) hostbridge.CommandResponse
}

// CommandHandlerFunc adapts a function to CommandHandler.
type CommandHandlerFunc func(ctx context.Context, clientIdentity string, req hostbridge.CommandRequest) hostbridge.CommandResponse

func (f CommandHandlerFunc) HandleCommand(ctx context.Context, clientIdentity string, req hostbridge.CommandRequest) hostbridge.CommandResponse {
	return f(ctx, clientIdentity, req)
}

// Server decodes and encodes one gob hostbridge command per connection.
type Server struct {
	Handler CommandHandler
}

func (s *Server) ServeConn(ctx context.Context, conn io.ReadWriteCloser) error {
	if conn == nil {
		return fmt.Errorf("missing connection")
	}
	defer conn.Close()
	if s == nil || s.Handler == nil {
		return fmt.Errorf("missing command handler")
	}
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	var req hostbridge.CommandRequest
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("decode command request: %w", err)
	}
	resp := s.Handler.HandleCommand(ctx, connectionClientIdentity(conn), req)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encode command response: %w", err)
	}
	return nil
}

func connectionClientIdentity(conn io.ReadWriteCloser) string {
	// Plain TCP connections intentionally have no client identity.
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return ""
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ""
	}
	return state.PeerCertificates[0].Subject.CommonName
}
