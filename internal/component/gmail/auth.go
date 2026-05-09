package gmail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const DefaultCallbackPort = 1455

var errMissingAuthMaterial = errors.New("missing gmail auth material")

func (c *Component) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	if c == nil {
		return fmt.Errorf("missing gmail component")
	}
	stdout = writerOrDiscard(stdout)
	stderr = writerOrDiscard(stderr)

	oauthConfig, configPath, err := c.loadOAuthConfig()
	if err != nil {
		fmt.Fprintln(stderr, gmailOAuthConfigHelp(c))
		return err
	}
	if callbackPort <= 0 {
		callbackPort = DefaultCallbackPort
	}
	if callbackTimeout <= 0 {
		callbackTimeout = 10 * time.Minute
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return fmt.Errorf("open gmail oauth callback listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	oauthConfig.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)
	state, err := randomState()
	if err != nil {
		return err
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{Handler: oauthCallbackHandler(state, codeCh, errCh)}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()
	defer func() { _ = server.Shutdown(context.Background()) }()

	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Fprintf(stdout, "gmail oauth client: %s\n", configPath)
	fmt.Fprintf(stdout, "open this URL to authenticate %s:\n%s\n", c.registration.Ref(), url)

	waitCtx, cancel := context.WithTimeout(ctx, callbackTimeout)
	defer cancel()
	var code string
	select {
	case <-waitCtx.Done():
		return waitCtx.Err()
	case err := <-errCh:
		return err
	case code = <-codeCh:
	}

	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange gmail oauth code: %w", err)
	}
	if err := c.saveToken(token); err != nil {
		return err
	}
	service, err := c.serviceFromToken(ctx, token)
	if err != nil {
		return err
	}
	c.Service = service
	profile, err := service.Users.GetProfile(c.userID()).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("check gmail profile: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(profile.EmailAddress)
	if c.mailboxEmail != "" {
		state, _ := c.loadState()
		state.MailboxEmail = c.mailboxEmail
		_ = c.saveState(state)
	}
	fmt.Fprintf(stdout, "gmail auth completed\naccount: %s\n", firstNonEmpty(c.mailboxEmail, c.userID()))
	return nil
}

func (c *Component) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	if c == nil {
		return fmt.Errorf("missing gmail component")
	}
	stdout = writerOrDiscard(stdout)
	stderr = writerOrDiscard(stderr)
	service, err := c.serviceFromStoredToken(ctx)
	if err != nil {
		if isMissingAuthMaterial(err) {
			fmt.Fprintf(stdout, "gmail auth: not authenticated\n%s\n", gmailOAuthConfigHelp(c))
			return nil
		}
		return err
	}
	c.Service = service
	profile, err := service.Users.GetProfile(c.userID()).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("check gmail profile: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(profile.EmailAddress)
	fmt.Fprintf(stdout, "gmail auth: authenticated\naccount: %s\n", firstNonEmpty(c.mailboxEmail, c.userID()))
	return nil
}

func (c *Component) serviceFromStoredToken(ctx context.Context) (*gmailapi.Service, error) {
	token, err := c.loadToken()
	if err != nil {
		return nil, err
	}
	return c.serviceFromToken(ctx, token)
}

func (c *Component) serviceFromToken(ctx context.Context, token *oauth2.Token) (*gmailapi.Service, error) {
	if token == nil {
		return nil, fmt.Errorf("%w: missing gmail token", errMissingAuthMaterial)
	}
	oauthConfig, _, err := c.loadOAuthConfig()
	if err != nil {
		return nil, err
	}
	client := oauthConfig.Client(ctx, token)
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}
	return service, nil
}

func (c *Component) loadOAuthConfig() (*oauth2.Config, string, error) {
	for _, path := range c.oauthConfigPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, path, fmt.Errorf("read gmail oauth client config %s: %w", path, err)
		}
		config, err := google.ConfigFromJSON(data, gmailapi.GmailReadonlyScope)
		if err != nil {
			return nil, path, fmt.Errorf("parse gmail oauth client config %s: %w", path, err)
		}
		return config, path, nil
	}
	return nil, "", fmt.Errorf("%w: gmail oauth client config not found", errMissingAuthMaterial)
}

func (c *Component) oauthConfigPaths() []string {
	var paths []string
	if c != nil {
		if path := strings.TrimSpace(c.oauthClientConfigPath); path != "" {
			paths = append(paths, path)
		}
		if home := strings.TrimSpace(c.home.Path); home != "" {
			paths = append(paths, filepath.Join(home, OAuthClientFilename))
		}
	}
	return uniqueNonEmpty(paths)
}

func (c *Component) loadToken() (*oauth2.Token, error) {
	path := c.tokenPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: gmail token not found", errMissingAuthMaterial)
		}
		return nil, fmt.Errorf("read gmail token %s: %w", path, err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("read gmail token %s: %w", path, err)
	}
	return &token, nil
}

func (c *Component) saveToken(token *oauth2.Token) error {
	if token == nil {
		return fmt.Errorf("missing gmail token")
	}
	path := c.tokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gmail token: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write gmail token: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit gmail token: %w", err)
	}
	return nil
}

func (c *Component) tokenPath() string {
	if c == nil {
		return TokenFilename
	}
	return filepath.Join(strings.TrimSpace(c.home.Path), TokenFilename)
}

func oauthCallbackHandler(wantState string, codeCh chan<- string, errCh chan<- error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		if got := query.Get("state"); got != wantState {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			sendCallbackError(errCh, fmt.Errorf("invalid gmail oauth state"))
			return
		}
		if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
			http.Error(w, oauthErr, http.StatusBadRequest)
			sendCallbackError(errCh, fmt.Errorf("gmail oauth error: %s", oauthErr))
			return
		}
		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			http.Error(w, "missing oauth code", http.StatusBadRequest)
			sendCallbackError(errCh, fmt.Errorf("missing gmail oauth code"))
			return
		}
		fmt.Fprintln(w, "Gmail authentication completed. You can close this tab.")
		sendCallbackCode(codeCh, code)
	})
	return mux
}

func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func gmailOAuthConfigHelp(c *Component) string {
	paths := ""
	if c != nil {
		paths = strings.Join(c.oauthConfigPaths(), " or ")
	}
	if strings.TrimSpace(paths) == "" {
		paths = OAuthClientFilename
	}
	return "gmail oauth client config missing; create a Google OAuth Desktop client and save its JSON at " + paths
}

func isMissingAuthMaterial(err error) bool {
	return errors.Is(err, errMissingAuthMaterial)
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func uniqueNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func sendCallbackError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func sendCallbackCode(ch chan<- string, code string) {
	select {
	case ch <- code:
	default:
	}
}
