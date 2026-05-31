package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/repository"
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
