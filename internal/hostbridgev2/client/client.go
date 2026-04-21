package client

import (
	"errors"
	"context"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"net"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridgev2"
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

func Do(ctx context.Context, address string, tlsDir string, req hostbridgev2.Request) (hostbridgev2.Response, error) {
	conn, err := Connect(ctx, address, tlsDir)
	if err != nil {
		return hostbridgev2.Response{}, err
	}
	defer conn.Close()
	return DoConn(conn, req)
}

func DoConn(conn net.Conn, req hostbridgev2.Request) (hostbridgev2.Response, error) {
	if conn == nil {
		return hostbridgev2.Response{}, fmt.Errorf("missing connection")
	}
	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		return hostbridgev2.Response{}, fmt.Errorf("encode request: %w", err)
	}
	var resp hostbridgev2.Response
	if err := gob.NewDecoder(conn).Decode(&resp); err != nil {
		return hostbridgev2.Response{}, fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
