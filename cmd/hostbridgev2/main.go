package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
	hostbridgev2 "github.com/bartdeboer/ctgbot/internal/hostbridge/v2"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func main() {
	if err := run(context.Background(), os.Args[1:], stdinForBody(os.Stdin), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func stdinForBody(stdin *os.File) io.Reader {
	if stdin == nil {
		return strings.NewReader("")
	}
	info, err := stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return strings.NewReader("")
	}
	return stdin
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHelp(stdout)
		return nil
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, buildassets.Version())
		return nil
	}
	var wantJSON bool
	args = append([]string(nil), args...)
	for len(args) > 0 {
		switch args[0] {
		case "--json":
			wantJSON = true
			args = args[1:]
		default:
			goto parsedFlags
		}
	}

parsedFlags:
	if len(args) == 0 {
		return fmt.Errorf("missing command")
	}
	body, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	httpClient, err := httpClientFromEnv()
	if err != nil {
		return err
	}
	client := &hostbridgev2.Client{
		BaseURL:     normalizeBaseURL(getenv("HOSTBRIDGE_V2_ADDR", "https://host.docker.internal:4569")),
		HTTPClient:  httpClient,
		BearerToken: os.Getenv("HOSTBRIDGE_V2_TOKEN"),
	}
	resp, err := client.Run(ctx, hostbridgev2.RunRequest{
		Command:   args,
		Stdin:     string(body),
		WantJSON:  wantJSON,
		SandboxID: sandboxIDFromEnv(),
	})
	if err != nil {
		if strings.TrimSpace(resp.Text) != "" {
			fmt.Fprintln(stdout, resp.Text)
		}
		return err
	}
	if strings.TrimSpace(resp.Text) != "" {
		fmt.Fprintln(stdout, resp.Text)
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("command failed with exit code %d", resp.ExitCode)
	}
	return nil
}

func httpClientFromEnv() (*http.Client, error) {
	tlsDir := strings.TrimSpace(os.Getenv("HOSTBRIDGE_V2_TLS_DIR"))
	if tlsDir == "" {
		tlsDir = strings.TrimSpace(os.Getenv("HOSTBRIDGE_TLS_DIR"))
	}
	if tlsDir == "" {
		return http.DefaultClient, nil
	}
	tlsConfig, err := hostbridgetls.LoadClientTLSConfig(tlsDir)
	if err != nil {
		return nil, fmt.Errorf("load hostbridge TLS config: %w", err)
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig.Clone()}}, nil
}

func sandboxIDFromEnv() modeluuid.UUID {
	value := strings.TrimSpace(os.Getenv("CTGBOT_SANDBOX_ID"))
	if value == "" {
		return modeluuid.UUID{}
	}
	id, err := modeluuid.Parse(value)
	if err != nil {
		return modeluuid.UUID{}
	}
	return id
}

func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		return value
	}
	return "https://" + value
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func printHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "usage: hostbridgev2 [--json] <command> [args...]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Sends a command through hostbridge HTTP v2.")
	fmt.Fprintln(stdout, "stdin is sent as the request body.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "environment:")
	fmt.Fprintln(stdout, "  HOSTBRIDGE_V2_ADDR     HTTPS base URL or address (default https://host.docker.internal:4569)")
	fmt.Fprintln(stdout, "  HOSTBRIDGE_V2_TLS_DIR  Optional TLS dir; falls back to HOSTBRIDGE_TLS_DIR")
	fmt.Fprintln(stdout, "  HOSTBRIDGE_V2_TOKEN    Optional bearer token for remote hostbridgev2")
	fmt.Fprintln(stdout, "  CTGBOT_SANDBOX_ID      Sandbox/thread id context header")
}
