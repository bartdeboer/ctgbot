package server

import (
	"context"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

type RunnerFactory func(clientIdentity string) chatcommands.Runner

type Server struct {
	Runner        chatcommands.Runner
	RunnerFactory RunnerFactory
}

func New(runner chatcommands.Runner) *Server {
	return &Server{Runner: runner}
}

func NewWithRunnerFactory(factory RunnerFactory) *Server {
	return &Server{RunnerFactory: factory}
}

func (s *Server) Handle(ctx context.Context, req hostbridge.Request) hostbridge.Response {
	return s.handle(ctx, "", req)
}

func (s *Server) ServeConn(ctx context.Context, conn io.ReadWriteCloser) error {
	if conn == nil {
		return fmt.Errorf("missing connection")
	}
	defer conn.Close()
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	var req hostbridge.Request
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	clientIdentity := connectionClientIdentity(conn)
	resp := s.handle(ctx, clientIdentity, req)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

func (s *Server) handle(ctx context.Context, clientIdentity string, req hostbridge.Request) hostbridge.Response {
	runner := s.Runner
	if s != nil && s.RunnerFactory != nil {
		runner = s.RunnerFactory(clientIdentity)
	}
	if runner == nil {
		return hostbridge.Response{Error: "hostbridge runner is unavailable"}
	}
	result, err := runner.Execute(ctx, req.Request)
	if err != nil {
		return hostbridge.Response{Error: err.Error()}
	}
	return hostbridge.Response{Result: result}
}

func connectionClientIdentity(conn io.ReadWriteCloser) string {
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
