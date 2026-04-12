package hostbridge

type Operation string

const (
	OpRunCommand Operation = "run-command"
	OpSendFile   Operation = "send-file"
)

const MaxSendFileBytes = 50 * 1024 * 1024

type Request struct {
	Op      Operation
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int

	SandboxID string
	Filename  string
	Caption   string
	Content   []byte
}

type StreamKind uint8

const (
	StreamStdout StreamKind = 1
	StreamStderr StreamKind = 2
	StreamExit   StreamKind = 3
	StreamError  StreamKind = 4
)

type Frame struct {
	Kind     StreamKind
	Data     []byte
	ExitCode int
	Message  string
}
