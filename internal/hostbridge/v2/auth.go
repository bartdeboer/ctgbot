package v2

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type Authenticator interface {
	Authenticate(req *http.Request) (commandengine.Actor, error)
}

type StaticActorAuth struct {
	Actor commandengine.Actor
}

type MTLSClientAuth struct {
	Roles []simplerbac.Role
}

type BearerTokenAuth struct {
	Token string
	Actor commandengine.Actor
}

func (a StaticActorAuth) Authenticate(req *http.Request) (commandengine.Actor, error) {
	actor := a.Actor
	if actor.ID == "" {
		actor.ID = "hostbridgev2"
	}
	if len(actor.Roles) == 0 {
		actor.Roles = []simplerbac.Role{simplerbac.RoleAgent}
	}
	return actor, nil
}

func (a MTLSClientAuth) Authenticate(req *http.Request) (commandengine.Actor, error) {
	if req == nil || req.TLS == nil || len(req.TLS.PeerCertificates) == 0 {
		return commandengine.Actor{}, fmt.Errorf("missing mTLS client certificate")
	}
	cert := req.TLS.PeerCertificates[0]
	id := strings.TrimSpace(cert.Subject.CommonName)
	if id == "" {
		return commandengine.Actor{}, fmt.Errorf("missing mTLS client certificate identity")
	}
	return commandengine.Actor{ID: id, Roles: rolesOrAgent(a.Roles)}, nil
}

func (a BearerTokenAuth) Authenticate(req *http.Request) (commandengine.Actor, error) {
	if req != nil && req.URL != nil && strings.TrimSpace(req.URL.Query().Get("access_token")) != "" {
		return commandengine.Actor{}, fmt.Errorf("bearer token must be sent in the Authorization header")
	}
	token := strings.TrimSpace(a.Token)
	if token == "" {
		return commandengine.Actor{}, fmt.Errorf("missing bearer token configuration")
	}
	header := ""
	if req != nil {
		header = strings.TrimSpace(req.Header.Get("Authorization"))
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return commandengine.Actor{}, fmt.Errorf("missing bearer token")
	}
	received := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if subtle.ConstantTimeCompare([]byte(received), []byte(token)) != 1 {
		return commandengine.Actor{}, fmt.Errorf("invalid bearer token")
	}
	actor := a.Actor
	if actor.ID == "" {
		actor.ID = "remote-hostbridge"
	}
	if len(actor.Roles) == 0 {
		actor.Roles = []simplerbac.Role{simplerbac.RoleAgent}
	}
	return actor, nil
}

func rolesOrAgent(roles []simplerbac.Role) []simplerbac.Role {
	if len(roles) == 0 {
		return []simplerbac.Role{simplerbac.RoleAgent}
	}
	return append([]simplerbac.Role(nil), roles...)
}
