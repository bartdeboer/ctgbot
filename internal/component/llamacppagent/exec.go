package llamacppagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

type ExecRuntime interface {
	Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error
}

type OutputHandler interface {
	Send(ctx context.Context, payload message.OutboundPayload) error
}

type ToolloopTurnRequest struct {
	ProviderThreadID string
	Prompt           string
	Env              []string
	ResultRuntime    string
	ResultHost       string
	EventsRuntime    string
	EventsHost       string
}

type ToolloopTurnResult struct {
	Reply            string
	ProviderThreadID string
	Result           toolloop.Result
}

type Runner struct {
	Logger       *log.Logger
	EventPoll    time.Duration
	ReasoningMax int
}

func NewRunner(logger *log.Logger) *Runner {
	return &Runner{Logger: logger}
}

func (r *Runner) RunTurn(ctx context.Context, runtime ExecRuntime, output OutputHandler, request ToolloopTurnRequest) (ToolloopTurnResult, error) {
	if runtime == nil {
		return ToolloopTurnResult{}, fmt.Errorf("missing runtime")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return ToolloopTurnResult{}, fmt.Errorf("missing prompt")
	}
	if strings.TrimSpace(request.ResultHost) == "" || strings.TrimSpace(request.ResultRuntime) == "" {
		return ToolloopTurnResult{}, fmt.Errorf("missing result path")
	}

	args := BuildExecArgs(request)
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runtime.Exec(ctx, &stdoutBuf, io.MultiWriter(os.Stderr, &stderrBuf), args[0], args[1:]...)
	}()

	runErr := r.forwardEventsUntilDone(ctx, strings.TrimSpace(request.EventsHost), output, done)
	result, readErr := readToolloopResult(request.ResultHost)
	if runErr != nil {
		if readErr != nil {
			return ToolloopTurnResult{Result: result}, fmt.Errorf("%w\n%s\nread result: %v", runErr, trimExecOutput(stdoutBuf.String(), stderrBuf.String()), readErr)
		}
		return ToolloopTurnResult{ProviderThreadID: result.ConversationID, Result: result}, fmt.Errorf("%w\n%s", runErr, trimExecOutput(stdoutBuf.String(), stderrBuf.String()))
	}
	if readErr != nil {
		return ToolloopTurnResult{}, readErr
	}
	return ToolloopTurnResult{
		Reply:            strings.TrimSpace(result.Text),
		ProviderThreadID: strings.TrimSpace(result.ConversationID),
		Result:           result,
	}, nil
}

func BuildExecArgs(request ToolloopTurnRequest) []string {
	innerArgs := append([]string{}, request.Env...)
	innerArgs = append(innerArgs, "toolloop", "--output", strings.TrimSpace(request.ResultRuntime))
	if eventsPath := strings.TrimSpace(request.EventsRuntime); eventsPath != "" {
		innerArgs = append(innerArgs, "--events", eventsPath)
	}
	if providerThreadID := strings.TrimSpace(request.ProviderThreadID); providerThreadID != "" {
		innerArgs = append(innerArgs, "resume", providerThreadID)
	}
	innerArgs = append(innerArgs, "--", strings.TrimSpace(request.Prompt))
	return agentcommon.WrapWithPIDFile(append([]string{"env"}, innerArgs...))
}

func (r *Runner) forwardEventsUntilDone(ctx context.Context, path string, output OutputHandler, done <-chan error) error {
	if strings.TrimSpace(path) == "" {
		return <-done
	}
	poll := r.EventPoll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	var offset int64
	for {
		if err := r.forwardNewEvents(ctx, path, &offset, output); err != nil {
			r.logf("llamacppagent event forward failed: %v", err)
		}
		select {
		case err := <-done:
			if drainErr := r.forwardNewEvents(ctx, path, &offset, output); drainErr != nil {
				r.logf("llamacppagent event drain failed: %v", drainErr)
			}
			return err
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *Runner) forwardNewEvents(ctx context.Context, path string, offset *int64, output OutputHandler) error {
	events, err := readNewEvents(path, offset)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, event := range events {
		r.forwardEvent(ctx, output, event)
	}
	return nil
}

func readNewEvents(path string, offset *int64) ([]toolloop.Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if offset == nil {
		return nil, nil
	}
	if *offset > info.Size() {
		*offset = 0
	}
	if _, err := file.Seek(*offset, io.SeekStart); err != nil {
		return nil, err
	}

	var events []toolloop.Event
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			var event toolloop.Event
			if decodeErr := json.Unmarshal([]byte(line), &event); decodeErr == nil {
				events = append(events, event)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return events, err
		}
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return events, err
	}
	*offset = position
	return events, nil
}

func (r *Runner) forwardEvent(ctx context.Context, output OutputHandler, event toolloop.Event) {
	if output == nil || event.Type != "model.response" {
		return
	}
	text, _ := event.Data["reasoning_content"].(string)
	if strings.TrimSpace(text) == "" {
		text, _ = event.Data["reasoning_preview"].(string)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := output.Send(ctx, message.OutboundPayload{
		Role: coremodel.MessageRoleAgent,
		Kind: coremodel.MessageKindProgress,
		Text: message.TextMessage{Text: text},
	}); err != nil {
		r.logf("send llamacppagent reasoning message failed: %v", err)
	}
}

func trimExecOutput(stdout string, stderr string) string {
	return strings.TrimSpace(strings.Join([]string{strings.TrimSpace(stdout), strings.TrimSpace(stderr)}, "\n"))
}

func (r *Runner) logf(format string, args ...any) {
	if r != nil && r.Logger != nil {
		r.Logger.Printf(format, args...)
	}
}

type commandRuntime struct {
	runtime       runtimepkg.ThreadRuntime
	workspacePath string
	threadID      modeluuid.UUID
	commands      commandengine.CommandExecutor
}

func (r commandRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	return r.runtime.Exec(ctx, r.workspacePath, r.threadID, r.commands, stdout, stderr, name, args...)
}

type outputHandler struct {
	runtime component.TurnRuntime
}

func (h outputHandler) Send(ctx context.Context, payload message.OutboundPayload) error {
	if h.runtime == nil {
		return nil
	}
	return h.runtime.Send(ctx, payload)
}
