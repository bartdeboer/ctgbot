package client

import (
	"context"
	"crypto/tls"
	"net"
	"strings"

	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
)

func Connect(ctx context.Context, address string, tlsDir string) (net.Conn, error) {
	dialer := &net.Dialer{}
	if strings.TrimSpace(tlsDir) == "" {
		return dialer.DialContext(ctx, "tcp", address)
	}
	baseConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := hostbridgetls.LoadClientTLSConfig(tlsDir)
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
