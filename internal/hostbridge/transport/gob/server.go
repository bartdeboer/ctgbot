package gobtransport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
	"github.com/bartdeboer/ctgbot/internal/identity"
)

// Server decodes and encodes one gob hostbridge command per connection.
type Server struct {
	Handler transport.CommandHandler
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
	resp := s.Handler.HandleCommand(ctx, connectionPeerIdentity(conn), req)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encode command response: %w", err)
	}
	return nil
}

func connectionPeerIdentity(conn io.ReadWriteCloser) transport.PeerIdentity {
	// Plain TCP connections intentionally have no transport-derived identity.
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return transport.PeerIdentity{}
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return transport.PeerIdentity{TLS: true}
	}
	return peerIdentityFromCertificate(state.PeerCertificates[0])
}

func peerIdentityFromCertificate(cert *x509.Certificate) transport.PeerIdentity {
	if cert == nil {
		return transport.PeerIdentity{}
	}
	return transport.PeerIdentity{
		CommonName:        cert.Subject.CommonName,
		FingerprintSHA256: identity.Fingerprint(cert),
		TLS:               true,
	}
}
