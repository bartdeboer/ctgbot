package gobtransport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
	"github.com/bartdeboer/ctgbot/internal/identity"
)

const DefaultMaxCommandRequestBytes int64 = 16 << 20

var errCommandRequestTooLarge = errors.New("hostbridge command request too large")

// Server decodes and encodes one gob hostbridge command per connection.
type Server struct {
	Handler         transport.CommandHandler
	MaxRequestBytes int64
}

func (s *Server) ServeConn(ctx context.Context, conn io.ReadWriteCloser) error {
	if conn == nil {
		return fmt.Errorf("missing connection")
	}
	defer conn.Close()
	if s == nil || s.Handler == nil {
		return fmt.Errorf("missing command handler")
	}
	limit := s.MaxRequestBytes
	if limit <= 0 {
		limit = DefaultMaxCommandRequestBytes
	}
	bounded := &io.LimitedReader{R: conn, N: limit}
	dec := gob.NewDecoder(bounded)
	enc := gob.NewEncoder(conn)
	var req hostbridge.CommandRequest
	if err := dec.Decode(&req); err != nil {
		if bounded.N == 0 {
			return fmt.Errorf("decode command request: %w (limit %d bytes)", errCommandRequestTooLarge, limit)
		}
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
