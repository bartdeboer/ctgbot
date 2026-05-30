package v2

import (
	"crypto/tls"
	"net/http"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

type ServerConfig struct {
	Addr      string
	TLSConfig *tls.Config
	Source    commandengine.Source
	Auth      Authenticator
}

func NewServer(runner CommandRunner, cfg ServerConfig) *http.Server {
	handler := NewHandler(runner)
	handler.Source = cfg.Source
	handler.Auth = cfg.Auth
	return &http.Server{
		Addr:      cfg.Addr,
		TLSConfig: cfg.TLSConfig,
		Handler:   handler,
	}
}
