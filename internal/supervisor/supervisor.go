package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type RestartPolicy string

const (
	RestartNever    RestartPolicy = "never"
	RestartAlways   RestartPolicy = "always"
	RestartError    RestartPolicy = "error"
	RestartComplete RestartPolicy = "complete"
)

type Config struct {
	Name         string        `json:"name"`
	Workdir      string        `json:"workdir,omitempty"`
	Command      []string      `json:"command"`
	Env          []string      `json:"env,omitempty"`
	Restart      RestartPolicy `json:"restart,omitempty"`
	RestartDelay string        `json:"restart_delay,omitempty"`
	LogPath      string        `json:"log_path,omitempty"`
	PIDPath      string        `json:"pid_path,omitempty"`
}

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, fmt.Errorf("missing supervisor config path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	base := filepath.Dir(path)
	cfg = cfg.WithDefaults(base)
	return cfg, cfg.Validate()
}

func (c Config) WithDefaults(base string) Config {
	if c.Restart == "" {
		c.Restart = RestartError
	}
	if strings.TrimSpace(c.RestartDelay) == "" {
		c.RestartDelay = "1s"
	}
	if strings.TrimSpace(c.LogPath) == "" {
		c.LogPath = filepath.Join(base, "service.log")
	}
	if strings.TrimSpace(c.PIDPath) == "" {
		c.PIDPath = filepath.Join(base, "supervisor.pid")
	}
	return c
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("missing service name")
	}
	if len(c.Command) == 0 || strings.TrimSpace(c.Command[0]) == "" {
		return fmt.Errorf("missing service command")
	}
	switch c.Restart {
	case RestartNever, RestartAlways, RestartError, RestartComplete:
		return nil
	default:
		return fmt.Errorf("invalid restart policy %q", c.Restart)
	}
}

func RunFile(ctx context.Context, path string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	return Run(ctx, cfg)
}

func Run(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	delay, err := time.ParseDuration(cfg.RestartDelay)
	if err != nil || delay <= 0 {
		delay = time.Second
	}
	if err := os.MkdirAll(filepath.Dir(cfg.PIDPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.PIDPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		return err
	}
	defer os.Remove(cfg.PIDPath)

	logFile, err := openLog(cfg.LogPath)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(logFile, os.Stderr), "", log.LstdFlags)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	for {
		exit, err := runOnce(ctx, cfg, logFile, logger)
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		if !shouldRestart(cfg.Restart, exit, err) {
			if err != nil {
				return err
			}
			return nil
		}
		logger.Printf("service %s exited code=%d err=%v: restarting in %s", cfg.Name, exit, err, delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

func openLog(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return os.Stderr, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func runOnce(ctx context.Context, cfg Config, logFile *os.File, logger *log.Logger) (int, error) {
	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = strings.TrimSpace(cfg.Workdir)
	cmd.Env = append(os.Environ(), cfg.Env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	logger.Printf("starting service %s: %s", cfg.Name, strings.Join(cfg.Command, " "))
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err := cmd.Wait()
	if err == nil {
		logger.Printf("service %s exited code=0", cfg.Name)
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		logger.Printf("service %s exited code=%d", cfg.Name, code)
		return code, err
	}
	logger.Printf("service %s wait error: %v", cfg.Name, err)
	return -1, err
}

func shouldRestart(policy RestartPolicy, code int, err error) bool {
	switch policy {
	case RestartAlways:
		return true
	case RestartError:
		return err != nil || code != 0
	case RestartComplete:
		return err == nil && code == 0
	default:
		return false
	}
}
