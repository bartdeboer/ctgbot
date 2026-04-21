package client

import (
	"errors"
	"context"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"net"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
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

func Do(ctx context.Context, address string, tlsDir string, req hostbridge.Request) (hostbridge.Response, error) {
	conn, err := Connect(ctx, address, tlsDir)
	if err != nil {
		return hostbridge.Response{}, err
	}
	defer conn.Close()
	return DoConn(conn, req)
}

func DoConn(conn net.Conn, req hostbridge.Request) (hostbridge.Response, error) {
	if conn == nil {
		return hostbridge.Response{}, fmt.Errorf("missing connection")
	}
	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		return hostbridge.Response{}, fmt.Errorf("encode request: %w", err)
	}
	var resp hostbridge.Response
	if err := gob.NewDecoder(conn).Decode(&resp); err != nil {
		return hostbridge.Response{}, fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
