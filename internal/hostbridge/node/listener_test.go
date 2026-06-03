package node

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/repository"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestPairingEndpointReturnsIdentityAndTLSExporterCodeIsAvailable(t *testing.T) {
	id := testIdentity(t, "controller")
	server := httptest.NewTLSServer(&PairingHandler{Identity: id})
	defer server.Close()

	client := server.Client()
	resp, err := client.Post(server.URL+PairPath, "application/json", strings.NewReader(`{"display_name":"node"}`))
	if err != nil {
		t.Fatalf("post pairing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.TLS == nil {
		t.Fatal("missing response TLS state")
	}
	if code, err := identity.PairingCode(*resp.TLS); err != nil || code == "" {
		t.Fatalf("PairingCode() = %q, %v", code, err)
	}
	var pair PairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatalf("decode pair response: %v", err)
	}
	if pair.Fingerprint != id.Fingerprint || !strings.Contains(pair.CertificatePEM, "BEGIN CERTIFICATE") {
		t.Fatalf("pair response = %+v, want identity", pair)
	}
}

func TestTrustedControllerAuthRequiresStoredFingerprint(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemory()
	controllerID := testIdentity(t, "controller")
	otherID := testIdentity(t, "other")
	if err := store.TrustedControllers().Save(ctx, &coremodel.TrustedController{
		ControllerID:   controllerID.ID,
		DisplayName:    "controller",
		Fingerprint:    controllerID.Fingerprint,
		CertificatePEM: controllerID.CertificatePEM,
	}); err != nil {
		t.Fatalf("save trusted controller: %v", err)
	}
	auth := TrustedControllerAuth{Repository: store.TrustedControllers()}
	actor, err := auth.Authenticate(requestWithPeerCert(controllerID.Certificate))
	if err != nil {
		t.Fatalf("Authenticate trusted: %v", err)
	}
	if actor.ID != controllerID.Fingerprint || !actor.HasRole(simplerbac.RoleRoot) {
		t.Fatalf("actor = %+v", actor)
	}
	if _, err := auth.Authenticate(requestWithPeerCert(otherID.Certificate)); err == nil {
		t.Fatalf("Authenticate accepted untrusted controller")
	}
}

func testIdentity(t *testing.T, name string) identity.Identity {
	t.Helper()
	id, err := identity.NewManager(t.TempDir(), name).Ensure()
	if err != nil {
		t.Fatalf("Ensure identity: %v", err)
	}
	return id
}

func requestWithPeerCert(cert *x509.Certificate) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	return req
}

func TestCommandsEndpointRequiresTrustedControllerCertificate(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemory()
	controller := testIdentity(t, "controller")
	if err := store.TrustedControllers().Save(ctx, &coremodel.TrustedController{
		ControllerID:   controller.ID,
		DisplayName:    "controller",
		Fingerprint:    controller.Fingerprint,
		CertificatePEM: controller.CertificatePEM,
	}); err != nil {
		t.Fatalf("save trusted controller: %v", err)
	}

	handler := testCommandsHandler(t, store)
	resp := runGobHTTPCommand(t, handler, controller.Certificate, "hello")
	if resp.Error != "" {
		t.Fatalf("trusted command error = %q", resp.Error)
	}
	if got, want := resp.Result.Text, "controller:hello"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}

	untrusted := testIdentity(t, "untrusted")
	resp = runGobHTTPCommand(t, handler, untrusted.Certificate, "hello")
	if !strings.Contains(resp.Error, "untrusted controller") {
		t.Fatalf("untrusted command error = %q, want untrusted controller", resp.Error)
	}

	resp = runGobHTTPCommand(t, handler, nil, "hello")
	if !strings.Contains(resp.Error, "missing controller client certificate") {
		t.Fatalf("missing cert command error = %q, want missing cert", resp.Error)
	}
}

func testCommandsHandler(t *testing.T, store repository.Storage) http.Handler {
	t.Helper()
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[schemacommands.Echo](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.Echo) (commandengine.Result, error) {
		_ = ctx
		return commandengine.Result{Text: req.Context.Actor.Label + ":" + cmd.Text}, nil
	}); err != nil {
		t.Fatalf("register echo handler: %v", err)
	}
	server := hostbridgeserver.NewCommandServer(registry)
	server.Prepare = trustedControllerCommandPreparer(TrustedControllerAuth{Repository: store.TrustedControllers()})
	return &gobtransport.HTTPHandler{Handler: server}
}

func runGobHTTPCommand(t *testing.T, handler http.Handler, cert *x509.Certificate, text string) hostbridge.CommandResponse {
	t.Helper()
	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(hostbridge.CommandRequest{Request: commandengine.Request{Command: schemacommands.Echo{Text: text}}}); err != nil {
		t.Fatalf("encode command request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, CommandsPath, &body)
	req.TLS = &tls.ConnectionState{}
	if cert != nil {
		req.TLS.PeerCertificates = []*x509.Certificate{cert}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp hostbridge.CommandResponse
	if err := gob.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	return resp
}
