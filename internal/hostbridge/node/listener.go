package node

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	v2 "github.com/bartdeboer/ctgbot/internal/hostbridge/v2"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const PairPath = "/v2/pair"

type Listener struct {
	Addr     string
	Runner   commandengine.CommandRunner
	Storage  repository.Storage
	Identity identity.Identity
	Logger   *log.Logger
}

func (l *Listener) Run(ctx context.Context) error {
	if l == nil {
		return fmt.Errorf("missing node listener")
	}
	addr := strings.TrimSpace(l.Addr)
	if addr == "" {
		return fmt.Errorf("missing node listener address")
	}
	if l.Runner == nil {
		return fmt.Errorf("missing node command runner")
	}
	if l.Storage == nil {
		return fmt.Errorf("missing node storage")
	}
	ln, err := tls.Listen("tcp", addr, identity.TLSConfig(l.Identity))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	pairing := &PairingHandler{Identity: l.Identity, Logger: l.Logger}
	mux.Handle(PairPath, pairing)
	commandHandler := v2.NewHandler(l.Runner)
	commandHandler.Source = commandengine.SourceController
	commandHandler.Auth = TrustedControllerAuth{Repository: l.Storage.TrustedControllers()}
	mux.Handle("/v2/run/", commandHandler)

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	l.logf("node remote listener serving addr=%s", ln.Addr().String())
	err = srv.Serve(ln)
	if ctx.Err() != nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (l *Listener) logf(format string, args ...any) {
	if l != nil && l.Logger != nil {
		l.Logger.Printf(format, args...)
	}
}

type PairingHandler struct {
	Identity identity.Identity
	Logger   *log.Logger
	mu       sync.Mutex
	lastByIP map[string]time.Time
}

type PairRequest struct {
	NodeID         string `json:"node_id,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	CertificatePEM string `json:"certificate_pem,omitempty"`
}

type PairResponse struct {
	ControllerID   string `json:"controller_id"`
	DisplayName    string `json:"display_name"`
	Fingerprint    string `json:"fingerprint"`
	CertificatePEM string `json:"certificate_pem"`
}

func (h *PairingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.TLS == nil {
		http.Error(w, "pairing requires TLS", http.StatusBadRequest)
		return
	}
	if !h.allow(req.RemoteAddr) {
		http.Error(w, "pairing rate limited", http.StatusTooManyRequests)
		return
	}
	var pairReq PairRequest
	_ = json.NewDecoder(req.Body).Decode(&pairReq)
	code, err := identity.PairingCode(*req.TLS)
	if err == nil && h.Logger != nil {
		display := strings.TrimSpace(pairReq.DisplayName)
		if display == "" {
			display = strings.TrimSpace(pairReq.NodeID)
		}
		if display == "" {
			display = strings.TrimSpace(req.RemoteAddr)
		}
		h.Logger.Printf("pairing request from %s confirm_code=%s", display, code)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PairResponse{
		ControllerID:   h.Identity.ID,
		DisplayName:    h.Identity.DisplayName,
		Fingerprint:    h.Identity.Fingerprint,
		CertificatePEM: h.Identity.CertificatePEM,
	})
}

func (h *PairingHandler) allow(remoteAddr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lastByIP == nil {
		h.lastByIP = map[string]time.Time{}
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil || strings.TrimSpace(host) == "" {
		host = remoteAddr
	}
	now := time.Now()
	if last, ok := h.lastByIP[host]; ok && now.Sub(last) < time.Second {
		return false
	}
	h.lastByIP[host] = now
	return true
}

type TrustedControllerAuth struct {
	Repository repository.TrustedControllerRepository
}

func (a TrustedControllerAuth) Authenticate(req *http.Request) (commandengine.Actor, error) {
	if req == nil || req.TLS == nil || len(req.TLS.PeerCertificates) == 0 {
		return commandengine.Actor{}, fmt.Errorf("missing controller client certificate")
	}
	cert := req.TLS.PeerCertificates[0]
	fingerprint := identity.Fingerprint(cert)
	controller, err := a.Repository.GetByFingerprint(req.Context(), fingerprint)
	if err != nil {
		return commandengine.Actor{}, err
	}
	if controller == nil {
		return commandengine.Actor{}, fmt.Errorf("untrusted controller: %s", fingerprint)
	}
	label := strings.TrimSpace(controller.DisplayName)
	if label == "" {
		label = fingerprint
	}
	return commandengine.Actor{ID: fingerprint, Label: label, Roles: []simplerbac.Role{simplerbac.RoleRoot}}, nil
}

func TrustedControllerRecord(resp PairResponse) coremodel.TrustedController {
	return coremodel.TrustedController{
		ControllerID:   strings.TrimSpace(resp.ControllerID),
		DisplayName:    strings.TrimSpace(resp.DisplayName),
		Fingerprint:    strings.TrimSpace(resp.Fingerprint),
		CertificatePEM: strings.TrimSpace(resp.CertificatePEM),
	}
}
