package app

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	nodelistener "github.com/bartdeboer/ctgbot/internal/hostbridge/node"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type trustControllerCommand struct {
	URL string
	Yes bool
}

type showIdentityCommand struct{}

type listTrustedControllersCommand struct{}

type revokeTrustedControllerCommand struct{ Fingerprint string }

var trustedControllerFingerprintPattern = regexp.MustCompile(`^SHA256:[0-9a-f]{64}$`)

func trustCommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "identity show",
			Help:    "Show this ctgbot instance stable identity",
			Build: func(req *clir.Request) (any, error) {
				return showIdentityCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "trust-controller <url>",
			Help:    "Trust another ctgbot instance as a controller using interactive pairing",
			Build: func(req *clir.Request) (any, error) {
				fs := flag.NewFlagSet("trust-controller", flag.ContinueOnError)
				fs.SetOutput(io.Discard)
				yes := fs.Bool("yes", false, "Trust without interactive confirmation after displaying the pairing code")
				if err := fs.Parse(req.Extra); err != nil {
					return nil, err
				}
				return trustControllerCommand{URL: strings.TrimSpace(req.Params["url"]), Yes: *yes}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "trusted-controllers list",
			Help:    "List controller identities trusted by this ctgbot instance",
			Build: func(req *clir.Request) (any, error) {
				return listTrustedControllersCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "trusted-controllers revoke <fingerprint>",
			Help:    "Revoke a trusted controller identity by fingerprint",
			Build: func(req *clir.Request) (any, error) {
				fingerprint := strings.TrimSpace(req.Params["fingerprint"])
				if fingerprint == "" {
					return nil, fmt.Errorf("missing fingerprint")
				}
				return revokeTrustedControllerCommand{Fingerprint: fingerprint}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
	}
}

func registerTrustCommandHandlers(registry *commandengine.Registry, surface *cliCommandSurface) error {
	if err := commandengine.Register[trustControllerCommand](registry, surface.handleTrustController); err != nil {
		return err
	}
	if err := commandengine.Register[showIdentityCommand](registry, surface.handleShowIdentity); err != nil {
		return err
	}
	if err := commandengine.Register[listTrustedControllersCommand](registry, surface.handleListTrustedControllers); err != nil {
		return err
	}
	return commandengine.Register[revokeTrustedControllerCommand](registry, surface.handleRevokeTrustedController)
}

func (s *cliCommandSurface) handleShowIdentity(ctx context.Context, req commandengine.Request, cmd showIdentityCommand) (commandengine.Result, error) {
	_, _ = ctx, req
	_ = cmd
	manager, err := s.service.identityManager()
	if err != nil {
		return commandengine.Result{}, err
	}
	id, err := manager.Ensure()
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{
		"ctgbot identity",
		fmt.Sprintf("display_name: %s", id.DisplayName),
		fmt.Sprintf("fingerprint: %s", id.Fingerprint),
	}, "\n")}, nil
}

func (s *cliCommandSurface) handleTrustController(ctx context.Context, req commandengine.Request, cmd trustControllerCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.service == nil || s.service.Storage == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	manager, err := s.service.identityManager()
	if err != nil {
		return commandengine.Result{}, err
	}
	self, err := manager.Ensure()
	if err != nil {
		return commandengine.Result{}, err
	}
	pairURL := strings.TrimRight(strings.TrimSpace(cmd.URL), "/") + nodelistener.PairPath
	body, _ := json.Marshal(nodelistener.PairRequest{
		NodeID:         self.ID,
		DisplayName:    self.DisplayName,
		CertificatePEM: self.CertificatePEM,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, pairURL, bytes.NewReader(body))
	if err != nil {
		return commandengine.Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: identity.ClientTLSConfig(self, true)}}
	resp, err := client.Do(httpReq)
	if err != nil {
		return commandengine.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		detail, _ := io.ReadAll(resp.Body)
		return commandengine.Result{}, fmt.Errorf("pairing failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return commandengine.Result{}, fmt.Errorf("pairing response missing TLS state")
	}
	code, err := identity.PairingCode(*resp.TLS)
	if err != nil {
		return commandengine.Result{}, err
	}
	var pairResp nodelistener.PairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairResp); err != nil {
		return commandengine.Result{}, fmt.Errorf("decode pairing response: %w", err)
	}
	cert, err := identity.ParseCertificatePEM(pairResp.CertificatePEM)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("parse controller certificate: %w", err)
	}
	actualFingerprint := identity.Fingerprint(cert)
	if strings.TrimSpace(pairResp.Fingerprint) != actualFingerprint {
		return commandengine.Result{}, fmt.Errorf("controller fingerprint mismatch")
	}
	lines := []string{
		"controller pairing",
		fmt.Sprintf("display_name: %s", strings.TrimSpace(pairResp.DisplayName)),
		fmt.Sprintf("fingerprint: %s", actualFingerprint),
		fmt.Sprintf("confirm_code: %s", code),
	}
	if !cmd.Yes {
		if !stdinIsTerminal() {
			return commandengine.Result{}, fmt.Errorf("interactive confirmation requires a terminal; pass --yes to trust this controller non-interactively")
		}
		fmt.Println(strings.Join(lines, "\n"))
		fmt.Print("Trust this controller? [y/N] ")
		var answer string
		_, _ = fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			return commandengine.Result{Text: "controller not trusted"}, nil
		}
	}
	record := nodelistener.TrustedControllerRecord(pairResp)
	record.Fingerprint = actualFingerprint
	if err := s.service.Storage.TrustedControllers().Save(ctx, &record); err != nil {
		return commandengine.Result{}, err
	}
	lines = append(lines, "trusted: true")
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleListTrustedControllers(ctx context.Context, req commandengine.Request, cmd listTrustedControllersCommand) (commandengine.Result, error) {
	_, _ = req, cmd
	controllers, err := s.service.Storage.TrustedControllers().List(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(controllers) == 0 {
		return commandengine.Result{Text: "trusted controllers: none"}, nil
	}
	var lines []string
	for _, controller := range controllers {
		status := "active"
		if controller.RevokedAt != nil {
			status = "revoked"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", controller.Fingerprint, status, controller.DisplayName))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleRevokeTrustedController(ctx context.Context, req commandengine.Request, cmd revokeTrustedControllerCommand) (commandengine.Result, error) {
	_ = req
	if !validTrustedControllerFingerprint(cmd.Fingerprint) {
		return commandengine.Result{}, fmt.Errorf("invalid controller fingerprint %q: expected SHA256:<64 lowercase hex chars>", cmd.Fingerprint)
	}
	ok, err := s.service.Storage.TrustedControllers().RevokeByFingerprint(ctx, cmd.Fingerprint)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !ok {
		return commandengine.Result{}, fmt.Errorf("trusted controller not found: %s", cmd.Fingerprint)
	}
	return commandengine.Result{Text: "controller revoked"}, nil
}

func validTrustedControllerFingerprint(value string) bool {
	return trustedControllerFingerprintPattern.MatchString(strings.TrimSpace(value))
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (s *service) identityManager() (*identity.Manager, error) {
	cfg := s.AppConfig()
	if cfg == nil || strings.TrimSpace(cfg.Profile().Root()) == "" {
		return nil, fmt.Errorf("missing ctgbot profile root")
	}
	return identity.NewManager(filepath.Join(cfg.Profile().Root(), "identity"), ""), nil
}

func (s *service) InstanceIdentity(ctx context.Context) (identity.Identity, error) {
	_ = ctx
	manager, err := s.identityManager()
	if err != nil {
		return identity.Identity{}, err
	}
	return manager.Ensure()
}
