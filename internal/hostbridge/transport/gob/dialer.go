package gobtransport

import (
	"context"
	"crypto/tls"
	"net"
)

// Dialer opens plain TCP connections, or mTLS connections when TLSConfig is set.
type Dialer struct {
	TLSConfig *tls.Config
}

func (d *Dialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	nd := &net.Dialer{}
	if d == nil || d.TLSConfig == nil {
		return nd.DialContext(ctx, "tcp", address)
	}
	baseConn, err := nd.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	conn := tls.Client(baseConn, d.TLSConfig)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
