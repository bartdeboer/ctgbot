package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
)

func Listen(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

func ListenTLS(address string, tlsConfig *tls.Config) (net.Listener, error) {
	if tlsConfig == nil {
		return nil, fmt.Errorf("missing tls config")
	}
	return tls.Listen("tcp", address, tlsConfig)
}

func ServeCommandListener(ctx context.Context, ln net.Listener, srv *CommandServer) error {
	if ln == nil {
		return fmt.Errorf("missing listener")
	}
	if srv == nil {
		return fmt.Errorf("missing command server")
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go func() {
			_ = srv.ServeCommandConn(ctx, conn)
		}()
	}
}
