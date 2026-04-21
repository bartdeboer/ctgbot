package chatcommands

import (
	"context"
	"fmt"
	"strings"

	clir "github.com/bartdeboer/go-clir"
)

type Operation string

const (
	OperationCommand    Operation = "command"
	OperationRun        Operation = "run"
	OperationSendFile   Operation = "send_file"
	OperationSendText   Operation = "send_text"
	OperationConfigList Operation = "config_list"
	OperationConfigSet  Operation = "config_set"
)

type Request struct {
	Op      Operation
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int

	SandboxID   string
	Filename    string
	Caption     string
	ContentType string
	Content     []byte

	Text     string
	Fenced   bool
	Language string

	Setting string
	Value   string
}

type Result struct {
	Text        string
	ContentType string
	Filename    string
	Caption     string
	Content     []byte
	Fenced      bool
	Language    string
}

type Execution struct {
	Request Request
	Result  *Result
}

type executionKey struct{}

type ChatCommands struct {
	router *clir.Router
}

func New() *ChatCommands {
	return &ChatCommands{router: clir.New()}
}

func (c *ChatCommands) Routes(fn func(b *clir.Builder)) {
	if c == nil || fn == nil {
		return
	}
	c.router.Routes(fn)
}

func (c *ChatCommands) Router() *clir.Router {
	if c == nil {
		return nil
	}
	return c.router
}

func (c *ChatCommands) Handle(ctx context.Context, req Request) (Result, error) {
	if c == nil || c.router == nil {
		return Result{}, fmt.Errorf("chat commands are unavailable")
	}
	argv, err := argvFor(req)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	exec := &Execution{Request: req, Result: &result}
	runCtx := context.WithValue(ctx, executionKey{}, exec)
	if err := c.router.Run(runCtx, argv); err != nil {
		return Result{}, err
	}
	return result, nil
}

func ExecutionFrom(ctx context.Context) (*Execution, bool) {
	if ctx == nil {
		return nil, false
	}
	exec, ok := ctx.Value(executionKey{}).(*Execution)
	if !ok || exec == nil {
		return nil, false
	}
	return exec, true
}

func RequestFrom(ctx context.Context) (Request, bool) {
	exec, ok := ExecutionFrom(ctx)
	if !ok {
		return Request{}, false
	}
	return exec.Request, true
}

func ResultFrom(ctx context.Context) (*Result, bool) {
	exec, ok := ExecutionFrom(ctx)
	if !ok || exec.Result == nil {
		return nil, false
	}
	return exec.Result, true
}

func Reply(req *clir.Request, text string) error {
	result, err := resultFor(req)
	if err != nil {
		return err
	}
	result.Text = text
	return nil
}

func SetResult(req *clir.Request, next Result) error {
	result, err := resultFor(req)
	if err != nil {
		return err
	}
	*result = next
	return nil
}

func argvFor(req Request) ([]string, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("missing command")
	}
	argv := []string{command}
	argv = append(argv, req.Args...)
	return argv, nil
}

func resultFor(req *clir.Request) (*Result, error) {
	if req == nil {
		return nil, fmt.Errorf("missing request")
	}
	result, ok := ResultFrom(req.Context())
	if !ok {
		return nil, fmt.Errorf("missing chat command result context")
	}
	return result, nil
}
