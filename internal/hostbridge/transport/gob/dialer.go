package gobtransport

import (
	"context"
	"crypto/tls"
	"net"
	"strings"

	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
)

// Dialer opens plain TCP connections, or mTLS connections when TLSDir is set.
type Dialer struct {
	TLSDir string
}

func (d *Dialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	if strings.TrimSpace(d.TLSDir) == "" {
		return dialer.DialContext(ctx, "tcp", address)
	}
	baseConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := hostbridgetls.LoadClientTLSConfig(d.TLSDir)
	if err != nil {
		_ = baseConn.Close()
		return nil, err
	}
	conn := tls.Client(baseConn, tlsConfig)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
