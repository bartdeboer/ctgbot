package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	hbclient "github.com/bartdeboer/ctgbot/internal/hostbridge/client"
	hbprotocol "github.com/bartdeboer/ctgbot/internal/hostbridge/protocol"
	"github.com/bartdeboer/go-clir"
)

func main() {
	rawArgs := os.Args[1:]
	args := rawArgs
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHostbridgeHelp()
		return
	}
	if len(args) > 0 && !isHostbridgeManagementCommand(args[0]) {
		args = append([]string{"run"}, args...)
	}

	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("run <command>", "Run a whitelisted host command over the bridge socket", func(req *clir.Request) error {
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}

			payload := hbprotocol.Request{
				Op:      hostbridge.OpRunCommand,
				Command: req.Params["command"],
				Args:    req.Extra,
				Stdin:   stdinData,
				Timeout: 30,
			}
			return sendHostbridgeRequest(payload)
		})

		b.Handle("sendfile <path>", "Upload a file to the current Telegram chat/thread via the host bridge", func(req *clir.Request) error {
			fs := flag.NewFlagSet("hostbridge sendfile", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			caption := fs.String("caption", "", "Optional Telegram caption")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			payload, err := buildSendFileRequest(req.Params["path"], strings.TrimSpace(*caption))
			if err != nil {
				return err
			}
			return sendHostbridgeRequest(payload)
		})

		b.Handle("sendstdin", "Send stdin to the current Telegram chat/thread via the host bridge", func(req *clir.Request) error {
			fs := flag.NewFlagSet("hostbridge sendstdin", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			fenced := fs.Bool("fenced", false, "Wrap stdin in a fenced code block")
			language := fs.String("language", "", "Optional fence language; implies --fenced")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			payload, err := buildSendTextRequest(string(stdinData), *fenced, strings.TrimSpace(*language))
			if err != nil {
				return err
			}
			return sendHostbridgeRequest(payload)
		})
	})

	if err := r.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHostbridgeHelp()
		os.Exit(1)
	}
}

func isHostbridgeManagementCommand(arg string) bool {
	switch arg {
	case "", "run", "sendfile", "sendstdin":
		return true
	default:
		return false
	}
}

func printHostbridgeHelp() {
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "environment:")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_ADDR     TCP address (default host.docker.internal:4567)")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_TLS_DIR  Optional directory containing ca.crt, client.crt, client.key")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  hostbridge ls -la")
	fmt.Fprintln(os.Stdout, "  hostbridge sendfile /workspace/out/report.pdf --caption \"Weekly report\"")
	fmt.Fprintln(os.Stdout, "  git diff | hostbridge sendstdin --fenced --language diff")
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func sendHostbridgeRequest(payload hbprotocol.Request) error {
	address := getenv("HOSTBRIDGE_ADDR", "host.docker.internal:4567")
	tlsDir := getenv("HOSTBRIDGE_TLS_DIR", "")
	return hbclient.SendRequest(address, tlsDir, payload, os.Stdout, os.Stderr)
}

func buildSendFileRequest(path string, caption string) (hbprotocol.Request, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing file path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return hbprotocol.Request{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return hbprotocol.Request{}, fmt.Errorf("not a regular file: %s", path)
	}
	if info.Size() > hostbridge.MaxSendFileBytes {
		return hbprotocol.Request{}, fmt.Errorf("file exceeds %d byte limit: %s", hostbridge.MaxSendFileBytes, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return hbprotocol.Request{}, fmt.Errorf("read file %s: %w", path, err)
	}

	sandboxID := getenv("CTGBOT_SANDBOX_ID", "")
	if strings.TrimSpace(sandboxID) == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing CTGBOT_SANDBOX_ID")
	}

	return hbprotocol.Request{
		Op:        hbprotocol.OpSendFile,
		Timeout:   30,
		SandboxID: sandboxID,
		Filename:  filepath.Base(path),
		Caption:   strings.TrimSpace(caption),
		Content:   content,
	}, nil
}

func buildSendTextRequest(text string, fenced bool, language string) (hbprotocol.Request, error) {
	sandboxID := getenv("CTGBOT_SANDBOX_ID", "")
	if strings.TrimSpace(sandboxID) == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing CTGBOT_SANDBOX_ID")
	}
	if strings.TrimSpace(language) != "" {
		fenced = true
	}
	if text == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing stdin content")
	}
	payloadText := wrapSendText(text, fenced, language)
	return hbprotocol.Request{
		Op:        hbprotocol.OpSendText,
		Timeout:   30,
		SandboxID: sandboxID,
		Text:      payloadText,
		Fenced:    fenced,
		Language:  strings.TrimSpace(language),
	}, nil
}

func wrapSendText(text string, fenced bool, language string) string {
	if !fenced {
		return text
	}
	language = strings.TrimSpace(language)
	text = strings.TrimRight(text, "\n")
	if language == "" {
		return "```\n" + text + "\n```"
	}
	return "```" + language + "\n" + text + "\n```"
}
