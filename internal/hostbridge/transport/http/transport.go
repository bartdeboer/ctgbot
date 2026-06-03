package httptransport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

const defaultMaxResponseBytes int64 = 16 << 20

// ByteTransport sends one opaque command payload as an HTTP POST body.
// It intentionally knows nothing about the command codec carried in that body.
type ByteTransport struct {
	URL              string
	Client           *http.Client
	MaxResponseBytes int64
}

var _ transport.ByteTransport = (*ByteTransport)(nil)

func (t *ByteTransport) Send(ctx context.Context, payload []byte) ([]byte, error) {
	if t == nil {
		return nil, fmt.Errorf("missing http byte transport")
	}
	url := strings.TrimSpace(t.URL)
	if url == "" {
		return nil, fmt.Errorf("missing http byte transport url")
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limit := t.MaxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if readErr != nil {
		return nil, fmt.Errorf("read http byte response: %w", readErr)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("http byte response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http byte transport failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
