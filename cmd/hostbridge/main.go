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
			contentType := fs.String("type", "", "Optional MIME-like content type")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			payload, err := buildSendFileRequest(req.Params["path"], strings.TrimSpace(*caption), strings.TrimSpace(*contentType))
			if err != nil {
				return err
			}
			return sendHostbridgeRequest(payload)
		})

		b.Handle("config list", "List settings available through the host bridge", func(req *clir.Request) error {
			payload, err := buildConfigListRequest()
			if err != nil {
				return err
			}
			return sendHostbridgeRequest(payload)
		})

		b.Handle("config set <name> <value>", "Set a policy-controlled config value through the host bridge", func(req *clir.Request) error {
			payload, err := buildConfigSetRequest(req.Params["name"], req.Params["value"])
			if err != nil {
				return err
			}
			return sendHostbridgeRequest(payload)
		})

		b.Handle("sendstdin", "Send stdin to the current Telegram chat/thread via the host bridge", func(req *clir.Request) error {
			fs := flag.NewFlagSet("hostbridge sendstdin", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			fenced := fs.Bool("fenced", false, "Wrap stdin in a fenced code block (legacy)")
			language := fs.String("language", "", "Optional fence language; implies --fenced (legacy)")
			contentType := fs.String("type", "text/plain", "Optional MIME-like content type")
			syntax := fs.String("syntax", "", "Optional syntax hint for fenced text")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			payload, err := buildSendTextRequest(string(stdinData), strings.TrimSpace(*contentType), *fenced, strings.TrimSpace(*language), strings.TrimSpace(*syntax))
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
	case "", "run", "sendfile", "sendstdin", "config":
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
	fmt.Fprintln(os.Stdout, "  git diff | hostbridge sendstdin --type text/plain --syntax diff")
	fmt.Fprintln(os.Stdout, "  hostbridge config list")
	fmt.Fprintln(os.Stdout, "  hostbridge config set chat.process_tools_enabled true")
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

func buildSendFileRequest(path string, caption string, contentType string) (hbprotocol.Request, error) {
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
		Op:          hbprotocol.OpSendFile,
		Timeout:     30,
		SandboxID:   sandboxID,
		Filename:    filepath.Base(path),
		Caption:     strings.TrimSpace(caption),
		ContentType: strings.TrimSpace(contentType),
		Content:     content,
	}, nil
}

func buildSendTextRequest(text string, contentType string, fenced bool, legacyLanguage string, syntax string) (hbprotocol.Request, error) {
	sandboxID := getenv("CTGBOT_SANDBOX_ID", "")
	if strings.TrimSpace(sandboxID) == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing CTGBOT_SANDBOX_ID")
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if contentType == "" {
		contentType = "text/plain"
	}
	if strings.TrimSpace(legacyLanguage) != "" {
		fenced = true
		if strings.TrimSpace(syntax) == "" {
			syntax = strings.TrimSpace(legacyLanguage)
		}
	}
	if text == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing stdin content")
	}
	if contentType == "text/markdown" && strings.TrimSpace(syntax) != "" {
		return hbprotocol.Request{}, fmt.Errorf("--syntax is not supported with text/markdown")
	}
	payloadText := wrapSendText(text, fenced || strings.TrimSpace(syntax) != "", syntax)
	return hbprotocol.Request{
		Op:          hbprotocol.OpSendText,
		Timeout:     30,
		SandboxID:   sandboxID,
		Text:        payloadText,
		Fenced:      fenced || strings.TrimSpace(syntax) != "",
		Language:    strings.TrimSpace(syntax),
		ContentType: contentType,
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

func buildConfigListRequest() (hbprotocol.Request, error) {
	sandboxID := getenv("CTGBOT_SANDBOX_ID", "")
	if strings.TrimSpace(sandboxID) == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing CTGBOT_SANDBOX_ID")
	}
	return hbprotocol.Request{Op: hbprotocol.OpConfigList, Timeout: 30, SandboxID: sandboxID}, nil
}

func buildConfigSetRequest(name string, value string) (hbprotocol.Request, error) {
	sandboxID := getenv("CTGBOT_SANDBOX_ID", "")
	if strings.TrimSpace(sandboxID) == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing CTGBOT_SANDBOX_ID")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return hbprotocol.Request{}, fmt.Errorf("missing setting name")
	}
	value = strings.TrimSpace(value)
	return hbprotocol.Request{Op: hbprotocol.OpConfigSet, Timeout: 30, SandboxID: sandboxID, Setting: name, Value: value}, nil
}
