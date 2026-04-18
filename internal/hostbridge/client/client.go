package client

import (
	"crypto/tls"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge/protocol"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
)

func Connect(address string, tlsDir string) (net.Conn, error) {
	if strings.TrimSpace(tlsDir) == "" {
		return net.Dial("tcp", address)
	}
	tlsConfig, err := hostbridgetls.LoadClientTLSConfig(tlsDir)
	if err != nil {
		return nil, err
	}
	return tls.Dial("tcp", address, tlsConfig)
}

func SendRequest(address string, tlsDir string, payload protocol.Request, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	conn, err := Connect(address, tlsDir)
	if err != nil {
		if strings.TrimSpace(tlsDir) != "" {
			return fmt.Errorf("connect tls %s: %w", address, err)
		}
		return fmt.Errorf("connect tcp %s: %w", address, err)
	}
	defer conn.Close()

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	for {
		var frame protocol.Frame
		if err := dec.Decode(&frame); err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		switch frame.Kind {
		case protocol.StreamStdout:
			if _, err := stdout.Write(frame.Data); err != nil {
				return err
			}
		case protocol.StreamStderr:
			if _, err := stderr.Write(frame.Data); err != nil {
				return err
			}
		case protocol.StreamError:
			return errors.New(frame.Message)
		case protocol.StreamExit:
			if frame.ExitCode != 0 {
				if f, ok := stdout.(*os.File); ok && f == os.Stdout {
					os.Exit(frame.ExitCode)
				}
				return fmt.Errorf("remote command exited with code %d", frame.ExitCode)
			}
			return nil
		default:
			return fmt.Errorf("unknown frame kind: %d", frame.Kind)
		}
	}
}
