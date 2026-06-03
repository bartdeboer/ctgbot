package client

import (
	"context"
	"net"

	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
)

func Connect(ctx context.Context, address string, tlsDir string) (net.Conn, error) {
	return (&gobtransport.Dialer{TLSDir: tlsDir}).Dial(ctx, address)
}
