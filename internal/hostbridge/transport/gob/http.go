package gobtransport

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

const defaultMaxHTTPCommandBytes int64 = 16 << 20

// HTTPHandler carries the existing gob command protocol over one HTTP request.
// The HTTP layer is transport only: gob still owns command encoding, and the
// CommandHandler still owns authorization and execution.
type HTTPHandler struct {
	Handler     transport.CommandHandler
	MaxBodySize int64
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.Handler == nil {
		http.Error(w, "missing command handler", http.StatusInternalServerError)
		return
	}
	limit := h.MaxBodySize
	if limit <= 0 {
		limit = defaultMaxHTTPCommandBytes
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, limit+1))
	if err != nil {
		http.Error(w, fmt.Sprintf("read command request: %v", err), http.StatusBadRequest)
		return
	}
	if int64(len(body)) > limit {
		http.Error(w, fmt.Sprintf("command request exceeds %d bytes", limit), http.StatusRequestEntityTooLarge)
		return
	}
	var commandReq hostbridge.CommandRequest
	if err := gob.NewDecoder(bytes.NewReader(body)).Decode(&commandReq); err != nil {
		http.Error(w, fmt.Sprintf("decode command request: %v", err), http.StatusBadRequest)
		return
	}
	peer := transport.PeerIdentity{TLS: req.TLS != nil}
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		peer = peerIdentityFromCertificate(req.TLS.PeerCertificates[0])
	}
	resp := h.Handler.HandleCommand(req.Context(), peer, commandReq)
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("encode command response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(payload.Bytes())
}
