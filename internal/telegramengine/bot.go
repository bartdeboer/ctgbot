package telegramengine

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/providerengine"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clir"
)

type tgUpdateKey struct{}
type tgEventKey struct{}

type SessionStore interface {
	AutoMigrate(ctx context.Context) error
	GetActive(ctx context.Context, chatID int64, threadID int) (*ChatSession, error)
	Create(ctx context.Context, sess *ChatSession) error
	MarkStopped(ctx context.Context, id uint, lastErr string) error
	MarkInitialized(ctx context.Context, id uint) error
	MarkError(ctx context.Context, id uint, lastErr string) error
	MarkProviderThreadID(ctx context.Context, id uint, threadID string) error
}

type SessionRunner interface {
	PrepareSandbox(req providerengine.PrepareSandboxRequest) error
	SandboxSpec(req providerengine.SandboxSpecRequest) sandboxengine.Spec
	SendPrompt(req providerengine.PromptRequest, sbx sandboxengine.Sandbox) (providerengine.PromptResult, error)
}

type TelegramBot struct {
	API       TelegramAPI
	Updates   *UpdateStorage
	Sessions  SessionStore
	Executor  SessionRunner
	Sandboxes sandboxengine.Manager
	Dispatch  *Dispatcher
	Config    *appconfig.Config
	Logger    *log.Logger
	router    *clir.Router
}

func NewTelegramBot(api TelegramAPI, updates *UpdateStorage, sessions SessionStore, executor SessionRunner, sandboxes sandboxengine.Manager, cfg *appconfig.Config, logger *log.Logger) *TelegramBot {
	dispatcher := NewDispatcher()
	if sandboxes == nil {
		sandboxes = &sandboxengine.DockerManager{Logger: logger}
	}
	return &TelegramBot{
		API:       api,
		Updates:   updates,
		Sessions:  sessions,
		Executor:  executor,
		Sandboxes: sandboxes,
		Dispatch:  dispatcher,
		Config:    cfg,
		Logger:    logger,
	}
}

func (tb *TelegramBot) AutoMigrate(ctx context.Context) error {
	if tb.Updates != nil {
		if err := tb.Updates.AutoMigrate(ctx); err != nil {
			return err
		}
	}
	if tb.Sessions != nil {
		if err := tb.Sessions.AutoMigrate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) Run(ctx context.Context) error {
	if tb.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := tb.Config.EnsurePaths(); err != nil {
		return err
	}

	tb.router = tb.buildRouter()
	return tb.API.Run(ctx, tb.Config.PollTimeout(), func(cbCtx context.Context, u chatmodel.TelegramUpdate) {
		tb.handleUpdate(cbCtx, u)
	})
}

func (tb *TelegramBot) buildRouter() *clir.Router {
	r := clir.New()

	r.Routes(func(b *clir.Builder) {
		tg := clir.WithContext(b, func(req *clir.Request) (chatmodel.TelegramUpdate, error) {
			v := req.Context().Value(tgUpdateKey{})
			u, ok := v.(chatmodel.TelegramUpdate)
			if !ok {
				return chatmodel.TelegramUpdate{}, fmt.Errorf("missing TelegramUpdate in context")
			}
			return u, nil
		})

		tg.Handle("new", "Start a new Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			workspace := ""
			if len(req.Extra) > 0 {
				workspace = req.Extra[0]
			}
			return tb.handleNewConversation(req.Context(), u, workspace)
		})

		tg.Handle("stop", "Stop the active Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.handleStopConversation(req.Context(), u)
		})

		tg.Handle("status", "Show the active Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.handleStatus(req.Context(), u)
		})

		tg.Handle("help", "Show available commands", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.replyText(req.Context(), u, "Commands:\n/new [absolute-host-path]\n/status\n/stop\n/help\n\nAny non-command message is sent to the active Codex conversation.")
		})
	})

	return r
}

func (tb *TelegramBot) handleUpdate(ctx context.Context, u chatmodel.TelegramUpdate) {
	text := strings.TrimSpace(u.Text)
	if text == "" {
		return
	}

	event := u
	if tb.Updates != nil {
		if err := tb.Updates.Create(ctx, &event); err != nil {
			tb.logf("persisting telegram event failed (chat=%d msg=%d): %v", u.ChatID, u.MessageID, err)
		}
	}
	ctx = context.WithValue(ctx, tgEventKey{}, &event)
	defer tb.persistEvent(ctx)

	key := chatmodel.ChatKey{ChatID: u.ChatID, ThreadID: u.ThreadID}
	if err := tb.Dispatch.Run(ctx, key, func(runCtx context.Context) error {
		return tb.handleUpdateSerialized(runCtx, u, text)
	}); err != nil {
		tb.recordEventError(ctx, err)
		tb.logf("telegram update handling failed chat=%d thread=%d msg=%d err=%v", u.ChatID, u.ThreadID, u.MessageID, err)
	}
}

func (tb *TelegramBot) handleUpdateSerialized(ctx context.Context, u chatmodel.TelegramUpdate, text string) error {

	tb.logf("telegram update chat=%d thread=%d msg=%d user=%q text=%q", u.ChatID, u.ThreadID, u.MessageID, u.UserLabel(), text)

	chatLabel := strings.TrimSpace(u.ChatTitle)
	if chatLabel == "" {
		chatLabel = u.UserLabel()
	}
	if err := tb.Config.PersistChatID(u.ChatID, chatLabel); err != nil {
		tb.logf("persisting chatID failed (chat=%d): %v", u.ChatID, err)
	}
	if !tb.Config.ChatEnabled(u.ChatID) {
		tb.logf("ignoring update from disabled chat=%d title=%q", u.ChatID, chatLabel)
		return nil
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeTelegramCommand(text)
		if len(args) == 0 {
			return nil
		}
		cmdCtx := context.WithValue(ctx, tgUpdateKey{}, u)
		if err := tb.router.Run(cmdCtx, args); err != nil {
			tb.recordEventError(ctx, err)
			_ = tb.replyText(ctx, u, fmt.Sprintf("command error: %v", err))
		}
		return nil
	}

	if err := tb.handlePrompt(ctx, u, text); err != nil {
		tb.recordEventError(ctx, err)
		_ = tb.replyText(ctx, u, fmt.Sprintf("conversation error: %v", err))
	}
	return nil
}

func (tb *TelegramBot) handleNewConversation(ctx context.Context, u chatmodel.TelegramUpdate, workspace string) error {
	conv, err := tb.startConversation(ctx, u.ChatID, u.ThreadID, workspace, true)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost)
	return tb.replyText(ctx, u, msg)
}

func (tb *TelegramBot) handleStopConversation(ctx context.Context, u chatmodel.TelegramUpdate) error {
	conv, err := tb.Sessions.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation")
	}
	if err := tb.sandboxManager().Remove(ctx, conv.ContainerName); err != nil {
		return err
	}
	if err := tb.Sessions.MarkStopped(ctx, conv.ID, "stopped by /stop"); err != nil {
		return err
	}
	return tb.replyText(ctx, u, "conversation stopped")
}

func (tb *TelegramBot) handleStatus(ctx context.Context, u chatmodel.TelegramUpdate) error {
	conv, err := tb.Sessions.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation")
	}
	msg := fmt.Sprintf(
		"active conversation\ncontainer: %s\nworkspace: %s\ninitialized: %t",
		conv.ContainerName,
		conv.WorkspaceHost,
		conv.Initialized,
	)
	if strings.TrimSpace(conv.LastError) != "" {
		msg += "\nlast_error: " + conv.LastError
	}
	return tb.replyText(ctx, u, msg)
}

func (tb *TelegramBot) handlePrompt(ctx context.Context, u chatmodel.TelegramUpdate, prompt string) error {
	conv, err := tb.Sessions.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		conv, err = tb.startConversation(ctx, u.ChatID, u.ThreadID, "", false)
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost)
		if err := tb.replyText(ctx, u, msg); err != nil {
			return err
		}
	}

	if err := tb.ensureSandboxRuntime(ctx, conv); err != nil {
		return err
	}
	spec := tb.decorateSandboxSpec(conv, tb.Executor.SandboxSpec(tb.sandboxSpecRequest(conv)))
	sbx, created, err := tb.sandboxManager().Ensure(ctx, spec)
	if err != nil {
		return err
	}
	if created {
		if err := tb.Executor.PrepareSandbox(tb.prepareSandboxRequest(conv)); err != nil {
			return err
		}
	}
	defer func() {
		if err := tb.sandboxManager().Stop(context.Background(), conv.ContainerName); err != nil {
			tb.logf("stop conversation sandbox %s failed: %v", conv.ContainerName, err)
		}
	}()

	result, runErr := tb.Executor.SendPrompt(tb.promptRequest(conv, prompt), sbx)
	reply := result.Reply
	if reply != "" {
		reply = cleanTextForTelegram(reply)
	}
	if strings.TrimSpace(result.ProviderThreadID) != "" {
		conv.ProviderThreadID = result.ProviderThreadID
	}

	if conv.ID != 0 && !conv.Initialized && runErr == nil {
		conv.Initialized = true
		_ = tb.Sessions.MarkInitialized(ctx, conv.ID)
	}
	if conv.ID != 0 {
		if strings.TrimSpace(conv.ProviderThreadID) != "" {
			_ = tb.Sessions.MarkProviderThreadID(ctx, conv.ID, conv.ProviderThreadID)
		}
		lastErr := ""
		if runErr != nil {
			lastErr = runErr.Error()
		}
		_ = tb.Sessions.MarkError(ctx, conv.ID, lastErr)
	}

	if reply != "" {
		if err := tb.replyText(ctx, u, reply); err != nil {
			return err
		}
	}
	return runErr
}

func (tb *TelegramBot) startConversation(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*ChatSession, error) {
	current, err := tb.Sessions.GetActive(ctx, chatID, threadID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = tb.sandboxManager().Remove(ctx, current.ContainerName)
		_ = tb.Sessions.MarkStopped(ctx, current.ID, "replaced by /new")
	}

	conv, err := tb.newConversationSession(ctx, chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	if err := tb.Sessions.Create(ctx, conv); err != nil {
		_ = tb.sandboxManager().Remove(ctx, conv.ContainerName)
		return nil, err
	}
	return conv, nil
}

func (tb *TelegramBot) ensureSandboxRuntime(ctx context.Context, conv *ChatSession) error {
	if tb.Config == nil {
		return fmt.Errorf("missing config")
	}
	if _, err := tb.Config.EnsureChatRuntimePaths(conv.ChatID); err != nil {
		return err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(tb.Config.HostbridgeTLSRoot(), tb.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID), conv.ContainerName); err != nil {
		return fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	return nil
}

func (tb *TelegramBot) newConversationSession(ctx context.Context, chatID int64, threadID int, workspace string) (*ChatSession, error) {
	if tb.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := tb.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if _, err := tb.Config.EnsureChatRuntimePaths(chatID); err != nil {
		return nil, err
	}
	workspaceHostPath, err := tb.Config.ResolveChatWorkspaceHostPath(chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	conv := &ChatSession{
		ChatID:             chatID,
		ThreadID:           threadID,
		Active:             true,
		ProviderType:       "codex",
		ContainerName:      tb.Config.ChatContainerName(chatID, threadID),
		WorkspaceHost:      workspaceHostPath,
		HomeHost:           tb.Config.ChatCodexHomeDirByID(chatID),
		ThreadRuntimeHost:  tb.Config.ChatThreadsRoot(chatID),
		ContainerWorkspace: tb.Config.ContainerWorkspacePath(),
		ContainerHome:      tb.Config.ContainerHomePath(),
	}
	if err := tb.sandboxManager().Remove(ctx, conv.ContainerName); err != nil {
		tb.logf("ignoring stale sandbox cleanup error for %s: %v", conv.ContainerName, err)
	}
	tb.logf("conversation session prepared name=%s workspace=%s", conv.ContainerName, conv.WorkspaceHost)
	return conv, nil
}

func (tb *TelegramBot) prepareSandboxRequest(conv *ChatSession) providerengine.PrepareSandboxRequest {
	allowedCommands := hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(tb.Config.ChatHostbridgeAllowedCommandSpecs(conv.ChatID)))
	return providerengine.PrepareSandboxRequest{
		ProfilePath:         conv.HomeHost,
		WorkspacePath:       conv.WorkspaceHost,
		ContainerHome:       conv.ContainerHome,
		ContainerWorkspace:  conv.ContainerWorkspace,
		HostOS:              runtime.GOOS,
		HostbridgeAddr:      tb.Config.ContainerHostbridgeTCPAddr(),
		AllowedHostCommands: allowedCommands,
	}
}

func (tb *TelegramBot) sandboxSpecRequest(conv *ChatSession) providerengine.SandboxSpecRequest {
	return providerengine.SandboxSpecRequest{
		SandboxName:        conv.ContainerName,
		ProfilePath:        conv.HomeHost,
		WorkspacePath:      conv.WorkspaceHost,
		ContainerHome:      conv.ContainerHome,
		ContainerWorkspace: conv.ContainerWorkspace,
	}
}

func (tb *TelegramBot) promptRequest(conv *ChatSession, prompt string) providerengine.PromptRequest {
	return providerengine.PromptRequest{
		ProviderThreadID:   conv.ProviderThreadID,
		Prompt:             prompt,
		ContainerHome:      conv.ContainerHome,
		ContainerWorkspace: conv.ContainerWorkspace,
	}
}

func (tb *TelegramBot) decorateSandboxSpec(conv *ChatSession, spec sandboxengine.Spec) sandboxengine.Spec {
	spec.SecurityOpts = appendUnique(spec.SecurityOpts, "seccomp=unconfined")
	spec.Labels = copyStringMap(spec.Labels)
	spec.Labels["ctgbot.managed"] = "true"
	spec.Labels["ctgbot.chat_id"] = fmt.Sprintf("%d", conv.ChatID)
	spec.Labels["ctgbot.thread_id"] = fmt.Sprintf("%d", conv.ThreadID)
	spec.Env = append(spec.Env,
		"HOSTBRIDGE_ADDR="+tb.Config.ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR="+tb.Config.ContainerHostbridgeTLSDir(),
	)
	spec.Mounts = append(spec.Mounts, sandboxengine.Mount{
		Source:   tb.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID),
		Target:   tb.Config.ContainerHostbridgeTLSDir(),
		ReadOnly: true,
	})
	if runtime.GOOS == "linux" {
		spec.AddHosts = appendUnique(spec.AddHosts, "host.docker.internal:host-gateway")
	}
	return spec
}

func (tb *TelegramBot) sandboxManager() sandboxengine.Manager {
	if tb.Sandboxes == nil {
		tb.Sandboxes = &sandboxengine.DockerManager{Logger: tb.Logger}
	}
	return tb.Sandboxes
}

func appendUnique(slice []string, values ...string) []string {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		found := false
		for _, existing := range slice {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, value)
		}
	}
	return slice
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (tb *TelegramBot) replyText(ctx context.Context, u chatmodel.TelegramUpdate, text string) error {
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}
	tb.appendEventResponse(ctx, text)

	for _, chunk := range splitTelegramText(text, 3500) {
		if err := tb.API.SendMessage(ctx, u.ChatID, u.ThreadID, u.MessageID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) logf(format string, args ...any) {
	if tb.Logger != nil {
		tb.Logger.Printf(format, args...)
	}
}

func normalizeTelegramCommand(text string) []string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return nil
	}
	fields[0] = strings.TrimPrefix(fields[0], "/")
	if i := strings.Index(fields[0], "@"); i >= 0 {
		fields[0] = fields[0][:i]
	}
	return fields
}

func splitTelegramText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit], "\n")
		if cut <= 0 {
			cut = limit
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func cleanTextForTelegram(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func (tb *TelegramBot) appendEventResponse(ctx context.Context, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	if strings.TrimSpace(event.ResponseText) == "" {
		event.ResponseText = text
		return
	}
	event.ResponseText += "\n\n" + text
}

func (tb *TelegramBot) recordEventError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	event.ErrorText = err.Error()
}

func (tb *TelegramBot) persistEvent(ctx context.Context) {
	if tb.Updates == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil || event.ID == 0 {
		return
	}
	if err := tb.Updates.Save(ctx, event); err != nil {
		tb.logf("persisting telegram event result failed (id=%d): %v", event.ID, err)
	}
}
