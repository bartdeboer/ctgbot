package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

const (
	threadsPath   = "/v1/threads"
	threadsPrefix = "/v1/threads/"
)

type Authenticator interface {
	Authenticate(r *http.Request) (coremodel.Actor, error)
}

type Server struct {
	Service       *messaging.Service
	Authenticator Authenticator
	Inbound       component.ResolvedInboundHandler
}

func New(service *messaging.Service, auth Authenticator, inbound component.ResolvedInboundHandler) *Server {
	return &Server{
		Service:       service,
		Authenticator: auth,
		Inbound:       inbound,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	mux.Handle(threadsPath, s)
	mux.Handle(threadsPrefix, s)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		http.Error(w, "missing messaging server", http.StatusInternalServerError)
		return
	}
	if r == nil {
		http.Error(w, "missing request", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == threadsPath:
		s.handleListThreads(w, r)
		return
	case strings.HasPrefix(r.URL.Path, threadsPrefix):
		s.handleThreadSubresource(w, r)
		return
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	req := messaging.ListThreadsRequest{
		Limit: parsePositiveInt(r.URL.Query().Get("limit"), 50),
		Query: strings.TrimSpace(r.URL.Query().Get("query")),
	}
	threads, err := s.Service.ListThreads(r.Context(), actor, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"threads": threads,
	})
}

func (s *Server) handleThreadSubresource(w http.ResponseWriter, r *http.Request) {
	threadID, tail, err := splitThreadPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch {
	case r.Method == http.MethodGet && tail == "messages":
		s.handleListMessages(w, r, threadID)
		return
	case r.Method == http.MethodPost && tail == "messages":
		s.handleSendMessage(w, r, threadID)
		return
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request, threadID modeluuid.UUID) {
	actor, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	req := messaging.ListMessagesRequest{
		Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 100),
	}
	page, err := s.Service.ListMessages(r.Context(), actor, threadID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request, threadID modeluuid.UUID) {
	actor, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if s.Inbound == nil {
		writeError(w, http.StatusNotImplemented, "resolved inbound handler not configured")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode request body: "+err.Error())
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "missing text")
		return
	}
	targetChat, targetThread, err := s.Service.ThreadTarget(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !targetChat.Enabled {
		writeError(w, http.StatusBadRequest, "target chat is disabled: "+targetChat.ID.String())
		return
	}
	result, err := s.Inbound.HandleResolvedInbound(r.Context(), component.ResolvedInbound{
		Chat:   *targetChat,
		Thread: *targetThread,
		Payload: message.InboundPayload{
			ProviderType: "thread",
			Text:         message.TextMessage{Text: req.Text},
			Actor:        messaging.ResolveActor(actor),
		},
		PromptContext: &component.InboundPromptContext{
			Kind:      "Remote thread message",
			FromLabel: actor.Label,
			FromID:    actor.ID,
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": result.Inbound,
	})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (coremodel.Actor, bool) {
	if s.Service == nil {
		writeError(w, http.StatusNotImplemented, "messaging service not configured")
		return coremodel.Actor{}, false
	}
	if s.Authenticator == nil {
		writeError(w, http.StatusUnauthorized, "missing authenticator")
		return coremodel.Actor{}, false
	}
	actor, err := s.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return coremodel.Actor{}, false
	}
	return messaging.ResolveActor(actor), true
}

func splitThreadPath(path string) (modeluuid.UUID, string, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, threadsPrefix) {
		return modeluuid.Nil, "", fmt.Errorf("invalid thread path")
	}
	rest := strings.TrimPrefix(path, threadsPrefix)
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return modeluuid.Nil, "", fmt.Errorf("invalid thread path")
	}
	threadID, err := modeluuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return modeluuid.Nil, "", fmt.Errorf("invalid thread id: %w", err)
	}
	return threadID, strings.TrimSpace(parts[1]), nil
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, text string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(text)})
}

type StaticTokenAuthenticator struct {
	Token string
	Actor coremodel.Actor
}

func (a StaticTokenAuthenticator) Authenticate(r *http.Request) (coremodel.Actor, error) {
	if r == nil {
		return coremodel.Actor{}, fmt.Errorf("missing request")
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if strings.TrimSpace(a.Token) == "" {
		return coremodel.Actor{}, fmt.Errorf("messaging auth not configured")
	}
	if token != strings.TrimSpace(a.Token) {
		return coremodel.Actor{}, fmt.Errorf("unauthorized")
	}
	return messaging.ResolveActor(a.Actor), nil
}

func bearerToken(authorization string) string {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(authorization), strings.ToLower(prefix)) {
		return ""
	}
	return strings.TrimSpace(authorization[len(prefix):])
}
