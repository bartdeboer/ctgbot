package server

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/hostbridgev2"
)

type Server struct {
	Runner chatcommands.Runner
}

func New(runner chatcommands.Runner) *Server {
	return &Server{Runner: runner}
}

func (s *Server) Handle(ctx context.Context, req hostbridgev2.Request) hostbridgev2.Response {
	if s == nil || s.Runner == nil {
		return hostbridgev2.Response{Error: "hostbridge runner is unavailable"}
	}
	result, err := s.Runner.Execute(ctx, req.Request)
	if err != nil {
		return hostbridgev2.Response{Error: err.Error()}
	}
	return hostbridgev2.Response{Result: result}
}

func (s *Server) ServeConn(ctx context.Context, conn io.ReadWriteCloser) error {
	if conn == nil {
		return fmt.Errorf("missing connection")
	}
	defer conn.Close()
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	var req hostbridgev2.Request
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	resp := s.Handle(ctx, req)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
